package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const SchemaVersionV2 = "argus.telemetry_dashboard/v2"
const OperationsOverviewTemplate = "kubernetes.operations-overview"

type OperationalSource struct {
	Kind          string   `json:"kind"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	States        []string `json:"states,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}
type InventoryRuntime interface {
	AccessScope(context.Context, kubernetesworkbench.Actor, uuid.UUID) (kubernetesworkbench.Scope, error)
	Snapshot(context.Context, kubernetesworkbench.Actor, uuid.UUID, kubernetesworkbench.SnapshotInput) (kubernetesworkbench.Snapshot, error)
	Get(context.Context, kubernetesworkbench.Actor, uuid.UUID, kubernetesworkbench.ObjectInput) (kubernetesinventory.Object, error)
}

func operationalPanel(p Panel) bool { return p.DataSource != nil && p.DataSource.Kind != "telemetry" }
func validateOperationalPanel(spec Spec, p Panel, path string, issue func(string, string)) {
	if spec.SchemaVersion != SchemaVersionV2 {
		issue(path, "operational panels require dashboard v2")
	}
	if p.DataSource == nil {
		return
	}
	inventory := p.DataSource.Kind == "kubernetes_inventory"
	alerts := p.DataSource.Kind == "alert_instances"
	if !inventory && !alerts {
		issue(path, "unsupported read-only data source")
	}
	if inventory && !slices.Contains([]string{"resource_summary", "node_allocation", "resource_distribution", "resource_graph", "resource_table", "resource_events"}, p.Type) || alerts && !slices.Contains([]string{"alert_summary", "alert_list"}, p.Type) {
		issue(path, "visualization and data source are incompatible")
	}
	if inventory && p.Signal != "resources" || alerts && p.Signal != "alerts" {
		issue(path, "invalid operational signal")
	}
	if p.AuthoringMode != "builder" || len(p.Targets) > 0 || len(p.DetailQueryTargets) > 0 || len(p.Drilldowns) > 0 || len(p.LocalFilters) > 0 {
		issue(path, "operational panels use typed read-only settings, not arbitrary queries or drilldowns")
	}
	if p.SourceBinding.SourceType != p.DataSource.Kind || p.SourceBinding.CapabilityVersion != "v1" {
		issue(path, "invalid operational capability")
	}
	if p.DataSource.Limit < 0 || p.DataSource.Limit > 1000 || len(p.DataSource.ResourceTypes) > 20 || len(p.DataSource.States) > 12 {
		issue(path, "operational panel exceeds budget")
	}
	for _, key := range p.DataSource.ResourceTypes {
		if p.Type == "node_allocation" && key != "node" && key != "pod" || p.Type == "resource_events" && key != "event" {
			issue(path, "resource type is incompatible with this visualization")
		}
		if _, ok := kubernetesinventory.Lookup(key); !ok {
			issue(path, "unsupported Kubernetes resource type")
		}
	}
	for _, state := range p.DataSource.States {
		if !slices.Contains([]string{"unknown", "healthy", "configured", "progressing", "pending", "warning", "failed", "completed", "paused", "scaled_down", "terminating"}, state) {
			issue(path, "unsupported resource state")
		}
	}
	if len(p.ApplicableResourceTypes) == 0 {
		issue(path, "operational resource scope required")
	}
	for _, kind := range p.ApplicableResourceTypes {
		if kind != "kubernetes_cluster" && kind != "kubernetes_namespace" {
			issue(path, "operational panel needs Kubernetes scope")
		}
	}
}

func (r Runtime) operationalScope(ctx context.Context, a Actor, resources []ResourceScope, requested *dashboardparams.KubernetesScope) (dashboardparams.KubernetesScope, error) {
	out := dashboardparams.KubernetesScope{NamespaceIDs: []uuid.UUID{}}
	if requested != nil {
		out = *requested
	}
	var parent uuid.UUID
	namespaces := []uuid.UUID{}
	for _, ref := range resources {
		switch ref.Type {
		case "kubernetes_cluster":
			if parent != uuid.Nil && parent != ref.ID {
				return out, ErrInvalid
			}
			parent = ref.ID
		case "kubernetes_namespace":
			namespaces = append(namespaces, ref.ID)
		default:
			return out, ErrInvalid
		}
	}
	if len(namespaces) > 0 {
		rows, err := r.Store.Queries.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, NamespaceIds: namespaces, ClusterIds: []uuid.UUID{}})
		if err != nil {
			return out, err
		}
		if len(rows) != len(namespaces) {
			return out, ErrDenied
		}
		for _, ns := range rows {
			if parent != uuid.Nil && parent != ns.ClusterID {
				return out, ErrInvalid
			}
			parent = ns.ClusterID
		}
	}
	if parent == uuid.Nil {
		return out, ErrInvalid
	}
	if out.ClusterID != uuid.Nil && out.ClusterID != parent {
		return out, ErrDenied
	}
	out.ClusterID = parent
	if len(namespaces) > 0 {
		if len(out.NamespaceIDs) == 0 {
			out.NamespaceIDs = namespaces
		}
		for _, id := range out.NamespaceIDs {
			if !slices.Contains(namespaces, id) {
				return out, ErrDenied
			}
		}
	}
	return out, nil
}

type operationalRound struct {
	types          []string
	bytes          int64
	alerts         []map[string]any
	alertsComplete bool
	alertsLoaded   bool
	snapshot       *kubernetesworkbench.Snapshot
	err            error
	loaded         bool
}

func (r Runtime) executeOperationalData(ctx context.Context, a Actor, p Panel, result Execution, round *operationalRound) (TargetExecution, error) {
	target := TargetExecution{ID: "resources", Status: "success", ResultType: "resource_snapshot"}
	scope, err := r.operationalScope(ctx, a, result.Resources, result.KubernetesScope)
	if err != nil {
		target.Status = "error"
		target.Code = "KUBERNETES_SCOPE_REQUIRED"
		return target, nil
	}
	if p.DataSource.Kind == "alert_instances" {
		selected := map[uuid.UUID]bool{}
		for _, ref := range result.Resources {
			selected[ref.ID] = true
		}
		if slices.ContainsFunc(result.Resources, func(ref ResourceScope) bool { return ref.Type == "kubernetes_cluster" }) {
			namespaces, e := r.Store.Queries.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, NamespaceIds: []uuid.UUID{}, ClusterIds: []uuid.UUID{scope.ClusterID}})
			if e != nil {
				return target, e
			}
			for _, ns := range namespaces {
				selected[ns.ID] = true
			}
		}
		items := round.alerts
		if items == nil {
			items = []map[string]any{}
		}
		complete := round.alertsComplete
		cursor := ""
		for pageNumber := 0; !round.alertsLoaded && pageNumber < 10; pageNumber++ {
			page, e := (alerting.Service{Store: r.Store}).Instances(ctx, alerting.Actor(a), alerting.ListFilter{Limit: 100, Cursor: cursor}, false)
			if e != nil {
				target.Status = "error"
				target.Code = "ALERT_DATA_UNAVAILABLE"
				return target, nil
			}
			for _, item := range page.Items {
				match := false
				for _, id := range item.ResourceIDs {
					if selected[id] {
						match = true
					}
				}
				if match {
					items = append(items, map[string]any{"id": item.ID, "policy_id": item.PolicyID, "name": item.State.Name, "severity": item.Severity, "health": item.Health, "lifecycle": item.Lifecycle, "started_at": item.StartedAt})
				}
			}
			complete = page.NextCursor == nil
			if complete {
				break
			}
			cursor = *page.NextCursor
		}
		round.alertsLoaded = true
		round.alerts = items
		round.alertsComplete = complete
		limit := p.DataSource.Limit
		if limit == 0 {
			limit = 20
		}
		total := len(items)

		target.ResultType = "alert_instances"
		target.Data = map[string]any{"items": items, "total": total, "page_size": limit, "complete": complete, "observed_at": time.Now().UTC(), "cluster_id": scope.ClusterID}
		if !complete {
			target.Status = "partial"
		}
		return target, nil
	}
	if r.Inventory == nil {
		target.Status = "error"
		target.Code = "KUBERNETES_API_UNAVAILABLE"
		return target, nil
	}
	if !round.loaded {
		round.loaded = true
		input := kubernetesworkbench.SnapshotInput{Types: round.types, Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}, MaxItems: 2000}
		if len(scope.NamespaceIDs) > 0 {
			input.Namespaces = kubernetesworkbench.Selection{Mode: "explicit", IDs: scope.NamespaceIDs}
		}
		snapshot, e := r.Inventory.Snapshot(ctx, kubernetesworkbench.Actor(a), scope.ClusterID, input)
		round.err = e
		round.snapshot = &snapshot
	}
	if round.err != nil {
		if errors.Is(round.err, authorization.ErrAccessDenied) {
			return target, ErrDenied
		}
		target.Status = "error"
		target.Code = kubernetesinventory.ErrorCode(round.err)
		return target, nil
	}
	types := p.DataSource.ResourceTypes
	if len(types) == 0 {
		switch p.Type {
		case "resource_summary":
			types = []string{"node", "pod", "deployment", "statefulset", "daemonset", "job", "pvc", "event"}
		case "node_allocation":
			types = []string{"node", "pod"}
		case "resource_events":
			types = []string{"event"}
		}
	}
	snapshot := *round.snapshot
	snapshot.Errors = map[string]string{}
	for key, value := range round.snapshot.Errors {
		snapshot.Errors[key] = value
	}
	snapshot.Counts = map[string]map[string]int{}
	for key := range round.snapshot.Counts {
		snapshot.Counts[key] = map[string]int{"total": 0}
	}
	snapshot.Resources = slices.Clone(snapshot.Resources)
	snapshot.Relations = slices.Clone(snapshot.Relations)
	if p.Type == "resource_graph" {
		candidates := []kubernetesworkbench.ResourceReference{}
		for _, row := range snapshot.Resources {
			if slices.Contains([]string{"deployment", "statefulset", "daemonset", "cronjob", "job", "pod", "service", "pvc"}, row.Type) {
				candidates = append(candidates, row.ResourceReference)
			}
		}
		snapshot.FocusCandidates = candidates
		focus := scope.FocusUID
		if focus != "" {
			if !slices.ContainsFunc(snapshot.Resources, func(row kubernetesworkbench.SnapshotResource) bool { return row.UID == focus }) {
				target.Status = "error"
				target.Code = "KUBERNETES_OBJECT_IDENTITY_CHANGED"
				if !snapshot.Complete {
					target.Code = "KUBERNETES_RESULT_BUDGET_EXCEEDED"
				}
				return target, nil
			}
		} else {
			for _, kind := range []string{"deployment", "statefulset", "daemonset", "cronjob", "job", "pod", "service", "pvc"} {
				for _, row := range snapshot.Resources {
					if row.Type == kind {
						focus = row.UID
						break
					}
				}
				if focus != "" {
					break
				}
			}
		}
		snapshot.FocusUID = focus
		selected := map[string]bool{}
		if focus != "" {
			selected[focus] = true
		}
		for depth := 0; depth < 3; depth++ {
			next := map[string]bool{}
			for key, value := range selected {
				next[key] = value
			}
			for _, edge := range snapshot.Relations {
				if selected[edge.From] && (edge.Kind != "schedules" || edge.From == focus) {
					next[edge.To] = true
				}
				if selected[edge.To] {
					next[edge.From] = true
				}
			}
			selected = next
		}
		snapshot.Resources = slices.DeleteFunc(snapshot.Resources, func(row kubernetesworkbench.SnapshotResource) bool { return !selected[row.UID] })
		if len(snapshot.Resources) > 200 {
			snapshot.Resources = []kubernetesworkbench.SnapshotResource{}
			snapshot.Relations = []kubernetesworkbench.ResourceRelation{}
			snapshot.Complete = false
			snapshot.Errors = map[string]string{"graph": "KUBERNETES_RESULT_BUDGET_EXCEEDED"}
		}
	}
	if p.Type == "resource_events" {
		snapshot.Resources = slices.DeleteFunc(snapshot.Resources, func(row kubernetesworkbench.SnapshotResource) bool {
			if row.Type != "event" {
				return true
			}
			observed, e := time.Parse(time.RFC3339Nano, row.Summary.Fields["event_time"])
			return e == nil && (observed.Before(result.From) || observed.After(result.To))
		})
	}
	if len(types) > 0 || len(p.DataSource.States) > 0 {
		snapshot.Resources = slices.DeleteFunc(snapshot.Resources, func(row kubernetesworkbench.SnapshotResource) bool {
			return len(types) > 0 && !slices.Contains(types, row.Type) || len(p.DataSource.States) > 0 && !slices.Contains(p.DataSource.States, row.Summary.State)
		})
	}
	allowed := map[string]bool{}
	for _, row := range snapshot.Resources {
		allowed[row.UID] = true
		if snapshot.Counts[row.Type] == nil {
			snapshot.Counts[row.Type] = map[string]int{}
		}
		snapshot.Counts[row.Type]["total"]++
		snapshot.Counts[row.Type][row.Summary.State]++
	}
	snapshot.Relations = slices.DeleteFunc(snapshot.Relations, func(edge kubernetesworkbench.ResourceRelation) bool { return !allowed[edge.From] || !allowed[edge.To] })
	if len(types) > 0 {
		for key := range snapshot.Counts {
			if !slices.Contains(types, key) {
				delete(snapshot.Counts, key)
				delete(snapshot.Errors, key)
			}
		}
		snapshot.Complete = len(snapshot.Errors) == 0
	}
	if p.Type == "node_allocation" {
		for key := range snapshot.Errors {
			if key != "node" && key != "pod" {
				delete(snapshot.Errors, key)
			}
		}
		snapshot.Complete = len(snapshot.Errors) == 0
	}
	if !snapshot.Complete {
		target.Status = "partial"
	}
	target.Data = snapshot
	// No raw manifests or Secret values are included in operational results.
	encoded, _ := json.Marshal(target.Data)
	if len(encoded) > 4<<20 {
		target.Status = "error"
		target.Code = "KUBERNETES_RESULT_BUDGET_EXCEEDED"
		target.Data = nil
	}
	return target, nil
}
