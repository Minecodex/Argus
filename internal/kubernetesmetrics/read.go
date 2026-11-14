package kubernetesmetrics

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

type validationKey struct{}
type queryValidation struct {
	actor  telemetry.Actor
	scopes []observationscope.Snapshot
}

func (s Service) Read(ctx context.Context, a kubernetesworkbench.Actor, cluster uuid.UUID, in ReadInput) (Result, error) {
	out := Result{Items: []Entry{}, ObservedAt: s.now()}
	if !slices.Contains([]string{"cpu", "memory", "storage"}, in.Metric) {
		return out, kubernetesinventory.ErrInvalid
	}
	state, err := s.decode(a, cluster, in.Token)
	if err != nil {
		return out, err
	}
	if state.Type == "pvc" && in.Metric != "storage" {
		return out, kubernetesinventory.ErrUnsupported
	}
	ctx = context.WithValue(ctx, validationKey{}, &queryValidation{})
	if err = s.validate(ctx, a, state); err != nil {
		return out, err
	}
	out.To = state.Observed.Truncate(30 * time.Second)
	out.From = out.To.Add(-30 * time.Minute)
	out.Complete = state.Complete
	label := "resource_k8s_pod_uid"
	ids := []string{}
	for _, p := range state.Pods {
		ids = append(ids, p.UID)
		if p.Alias != "" {
			ids = append(ids, p.Alias)
		}
	}
	if state.Type == "node" {
		label = "resource_k8s_node_name"
		ids = nil
		for _, r := range state.Rows {
			if !r.Changed {
				ids = append(ids, r.Name)
			}
		}
	}
	if state.Type == "pvc" {
		return s.readPVCs(ctx, a, state, out)
	}
	usedMetric := map[string]string{"cpu": "k8s_pod_cpu_usage", "memory": "k8s_pod_memory_usage", "storage": "k8s_pod_filesystem_usage"}[in.Metric]
	if state.Type == "node" {
		usedMetric = map[string]string{"cpu": "k8s_node_cpu_usage", "memory": "k8s_node_memory_usage", "storage": "k8s_node_filesystem_usage"}[in.Metric]
	}
	selector := match(label, ids)
	groupLabel := label
	if state.Type != "node" {
		names, namespaces := []string{}, []string{}
		for _, p := range state.Pods {
			names = append(names, p.Name)
			namespaces = append(namespaces, p.Namespace)
		}
		if len(names) > 0 {
			selector = strings.TrimSuffix(selector, "}") + "," + strings.TrimPrefix(match("resource_k8s_pod_name", names), "{")
			selector = strings.TrimSuffix(selector, "}") + "," + strings.TrimPrefix(match("resource_k8s_namespace_name", namespaces), "{")
		}
		groupLabel = "resource_k8s_namespace_name,resource_k8s_pod_name," + label
	}
	used := map[string]series{}
	usedError := ""
	if len(ids) > 0 {
		expression := usageExpression(usedMetric, selector, groupLabel)
		if state.Type == "node" && in.Metric == "storage" {
			expression += " or " + component("max by ("+label+") (k8s_node_filesystem_capacity"+selector+")", "capacity")
		}
		used, err = s.query(ctx, a, state, "kubeletstats", expression, out.From, out.To)
		if err != nil {
			usedError = reason(err)
			out.Complete = false
		}
	}
	configuration := map[string]series{}
	configError := ""
	if state.Type != "node" && len(ids) > 0 {
		suffix := in.Metric
		if suffix == "storage" {
			suffix = "ephemeralstorage"
		}
		expression := configurationExpression(suffix, selector, groupLabel)
		configuration, err = s.query(ctx, a, state, "k8s_cluster", expression, out.From, out.To)
		if err != nil {
			configError = reason(err)
		}
	}
	for _, r := range state.Rows {
		trend := s.buildTrend(state, r, in.Metric, used, configuration, out.From, out.To)
		if r.Changed {
			trend.State = "identity_changed"
			trend.Reason = "KUBERNETES_OBJECT_IDENTITY_CHANGED"
		} else if usedError != "" {
			trend.State = stateFor(usedError)
			trend.Reason = usedError
		} else if state.PodError != "" && state.Type != "node" {
			trend.State = "partial"
			trend.Reason = state.PodError
		}
		if configError != "" && trend.State == "ready" {
			trend.Reason = configError
		}
		out.Items = append(out.Items, Entry{UID: r.UID, Trends: []Trend{trend}})
	}
	if in.Metric == "storage" && state.Type != "node" {
		s.appendVolumes(ctx, a, state, &out)
	}
	if err = s.validate(ctx, a, state); err != nil {
		return Result{}, err
	}
	return out, nil
}
func (s Service) validate(ctx context.Context, a kubernetesworkbench.Actor, state preparedState) error {
	scope, e := s.Inventory.AccessScope(ctx, a, state.Cluster)
	if e != nil {
		return e
	}
	if pagination.HashFilter(scope) != state.Scope {
		return authorization.ErrAccessDenied
	}
	if queries, ok := ctx.Value(validationKey{}).(*queryValidation); ok {
		for _, snapshot := range queries.scopes {
			if e := s.Telemetry.ValidateResolvedQueryScope(ctx, queries.actor, snapshot); e != nil {
				return authorization.ErrAccessDenied
			}
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, r := range state.Rows {
		if r.Changed {
			continue
		}
		if r.NamespaceID != uuid.Nil && !seen[r.NamespaceID] {
			seen[r.NamespaceID] = true
			_, e = s.Inventory.Get(ctx, a, state.Cluster, kubernetesworkbench.ObjectInput{Type: "namespace", Name: r.Namespace, UID: r.NamespaceUID})
			if e != nil {
				return e
			}
		}
	}
	return nil
}
func match(label string, ids []string) string {
	unique := map[string]bool{}
	values := []string{}
	for _, id := range ids {
		if id != "" && !unique[id] {
			unique[id] = true
			values = append(values, regexp.QuoteMeta(id))
		}
	}
	sort.Strings(values)
	return fmt.Sprintf("{%s=~%q}", label, strings.Join(values, "|"))
}
func component(expr, key string) string {
	return fmt.Sprintf(`label_replace((%s),"argus_trend_component",%q,"","")`, expr, key)
}
func usageExpression(metric, selector, label string) string {
	return component("max by ("+label+") ("+metric+selector+")", "used") + " or " + component("max by ("+label+") (timestamp("+metric+selector+"))", "sample_time")
}
func configurationExpression(metric, selector, label string) string {
	parts := []string{}
	for _, kind := range []string{"request", "limit"} {
		base := "max by (" + label + ",resource_k8s_container_name) (k8s_container_" + metric + "_" + kind + selector + ")"
		parts = append(parts, component("sum by ("+label+") ("+base+")", kind), component("count by ("+label+") ("+base+")", kind+"_count"))
	}
	return strings.Join(parts, " or ")
}
func (s Service) query(ctx context.Context, a kubernetesworkbench.Actor, state preparedState, source, expression string, from, to time.Time) (map[string]series, error) {
	if s.Telemetry.Engine == nil {
		return nil, telemetry.ErrUnavailable
	}
	actor, e := s.Telemetry.ResourceQueryActor(ctx, a.EnterpriseID, a.SubjectID, a.SubjectType, a.AuthorizationVersion)
	if e != nil {
		return nil, e
	}
	refs := metricResources(state)
	if len(refs) == 0 {
		return map[string]series{}, nil
	}
	snapshot, keys, e := s.Telemetry.ResolveStructuredQueryScope(ctx, actor, telemetry.QueryScope{Mode: "explicit", Resources: refs, SourceType: source}, "metrics")
	if e != nil {
		return nil, e
	}
	if len(keys) == 0 {
		return nil, telemetry.ErrQuerySourcesEmpty
	}
	result, e := s.Telemetry.Engine.ExecuteEngineQuery(ctx, queryengine.Request{CacheNamespace: "kubernetes_list_metrics/v1", Language: queryengine.LanguagePromQL, Expression: expression, Start: from, End: to, Step: 30 * time.Second, Scope: queryengine.Scope{Resources: snapshot.DataRefs(), Signature: snapshot.QuerySignature(), SourceKeys: keys, EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ResourceIDs: snapshot.DataIDs()}, Budget: queryengine.Budget{MaxScanBytes: 128 << 20, MaxRows: 20000, MaxSamples: 500000, MaxSeries: 4000, MaxResultBytes: 12 << 20, Timeout: 8 * time.Second}})
	if e != nil {
		return nil, e
	}
	if e = s.Telemetry.ValidateResolvedQueryScope(ctx, actor, snapshot); e != nil {
		return nil, e
	}
	if queries, ok := ctx.Value(validationKey{}).(*queryValidation); ok {
		queries.actor = actor
		queries.scopes = append(queries.scopes, snapshot)
	}
	if result.Meta.Partial {
		return nil, kubernetesinventory.ErrBudget
	}
	return parseSeries(result.Data)
}

// A Cluster grant may use its own source root, still narrowed by trusted UID,
// namespace and name selectors. Namespace-only actors never receive this root.
// This also preserves kubelet static-Pod UID aliases, which are not UUIDs.
func metricResources(state preparedState) []resourceref.Ref {
	if state.ClusterWide {
		return []resourceref.Ref{{Type: "kubernetes_cluster", ID: state.Cluster}}
	}
	refs := []resourceref.Ref{}
	seen := map[uuid.UUID]bool{}
	for _, r := range state.Rows {
		if !r.Changed && r.NamespaceID != uuid.Nil && !seen[r.NamespaceID] {
			refs = append(refs, resourceref.Ref{Type: "kubernetes_namespace", ID: r.NamespaceID})
			seen[r.NamespaceID] = true
		}
	}
	return refs
}
func reason(err error) string {
	if errors.Is(err, telemetry.ErrDenied) || errors.Is(err, authorization.ErrAccessDenied) {
		return "FORBIDDEN"
	}
	if errors.Is(err, telemetry.ErrQuerySourcesEmpty) {
		return "NO_SOURCE"
	}
	if errors.Is(err, kubernetesinventory.ErrBudget) || errors.Is(err, queryengine.ErrBudget) {
		return "INCOMPLETE"
	}
	return "QUERY_FAILED"
}
func stateFor(code string) string {
	switch code {
	case "FORBIDDEN":
		return "forbidden"
	case "NO_SOURCE":
		return "not_configured"
	case "INCOMPLETE":
		return "partial"
	}
	return "query_failed"
}
