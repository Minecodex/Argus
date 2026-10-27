package telemetry

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

type IngestMetricNamespaceResolver interface {
	ResolveMetricNamespace(context.Context, TrustedIdentity, SourceIdentity, string, string) (string, error)
}

type MetricNamespaceObject struct{ Type, UID string }

// Missing inventory proof does not invalidate a previously verified Cluster
// source. Such observations never acquire Namespace row authority.
var errNamespaceAttributionUnverified = errors.New("namespace attribution is unverified")

type IngestMetricObjectNamespaceResolver interface {
	ResolveMetricObjectNamespace(context.Context, TrustedIdentity, SourceIdentity, string, []MetricNamespaceObject) (string, error)
}

func (store PostgresIngestControl) ResolveMetricObjectNamespace(ctx context.Context, identity TrustedIdentity, source SourceIdentity, name string, objects []MetricNamespaceObject) (string, error) {
	if source.ResourceType != "kubernetes_cluster" || source.Type != "k8s_cluster" && source.Type != "kubeletstats" || name == "" || len(name) > 253 || len(objects) == 0 || len(objects) > 8 {
		return "", errIngestRejected
	}
	uid, unverified := "", false
	for _, object := range objects {
		// Kubernetes UID is opaque: static Pods can use a hash rather than a
		// UUID. Preserve its exact bytes and require the trusted inventory match
		// below; formatting a UID never confers Namespace authority.
		if !metricNamespaceObjectType(object.Type) || object.UID == "" || len(object.UID) > 128 || strings.IndexFunc(object.UID, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return "", errIngestRejected
		}
		actual, err := store.Queries.GetKubernetesMetricObjectIdentity(ctx, db.GetKubernetesMetricObjectIdentityParams{EnterpriseID: identity.EnterpriseID, ClusterID: source.ResourceID, ObjectUid: object.UID})
		if errors.Is(err, pgx.ErrNoRows) {
			unverified = true
			continue // Still check every other identity for known contradictions.
		}
		if err != nil {
			return "", controlReadError(err)
		}
		if actual.ObjectType != object.Type || actual.NamespaceName != name || uid != "" && uid != actual.NamespaceUid {
			return "", errIngestRejected
		}
		if !actual.NamespaceActive {
			unverified = true
		}
		uid = actual.NamespaceUid
	}
	if unverified {
		return "", errNamespaceAttributionUnverified
	}
	return uid, nil
}

func metricNamespaceObjectType(kind string) bool {
	switch kind {
	case "pod", "deployment", "replicaset", "daemonset", "statefulset", "job", "cronjob", "pvc":
		return true
	default:
		return false
	}
}

func (store PostgresIngestControl) ResolveMetricNamespace(ctx context.Context, identity TrustedIdentity, source SourceIdentity, name, uid string) (string, error) {
	if source.ResourceType != "kubernetes_cluster" || source.Type != "k8s_cluster" && source.Type != "kubeletstats" || name == "" || len(name) > 253 {
		return "", errIngestRejected
	}
	if parsed, e := uuid.Parse(uid); e != nil || parsed == uuid.Nil || parsed.String() != uid {
		return "", ErrSourceAwaitingApply
	}
	row, err := store.Queries.ResolveKubernetesNamespaceResource(ctx, db.ResolveKubernetesNamespaceResourceParams{EnterpriseID: identity.EnterpriseID, ClusterID: source.ResourceID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSourceAwaitingApply
	}
	if err != nil {
		return "", controlReadError(err)
	}
	if row.NamespaceUid != uid {
		return "", errIngestRejected
	}
	return row.NamespaceUid, nil
}

func (server *IngestServer) metricNamespace(ctx context.Context, identity TrustedIdentity, source SourceIdentity, attrs []*commonpb.KeyValue) (string, error) {
	if source.ResourceType != "kubernetes_cluster" || source.Type != "k8s_cluster" && source.Type != "kubeletstats" {
		return "", nil
	}
	values := map[string]string{}
	objectKeys := map[string]string{"k8s.pod.uid": "pod", "k8s.deployment.uid": "deployment", "k8s.replicaset.uid": "replicaset", "k8s.daemonset.uid": "daemonset", "k8s.statefulset.uid": "statefulset", "k8s.job.uid": "job", "k8s.cronjob.uid": "cronjob", "k8s.persistentvolumeclaim.uid": "pvc"}
	objects := []MetricNamespaceObject{}
	for _, attr := range attrs {
		if attr == nil {
			return "", errIngestRejected
		}
		if attr.Key != "k8s.namespace.name" && attr.Key != "k8s.namespace.uid" && objectKeys[attr.Key] == "" {
			continue
		}
		if _, duplicate := values[attr.Key]; duplicate {
			return "", errIngestRejected
		}
		value, ok := attr.Value.GetValue().(*commonpb.AnyValue_StringValue)
		if !ok {
			return "", errIngestRejected
		}
		values[attr.Key] = value.StringValue
		if kind := objectKeys[attr.Key]; kind != "" {
			objects = append(objects, MetricNamespaceObject{Type: kind, UID: value.StringValue})
		}
	}
	if values["k8s.namespace.name"] == "" && values["k8s.namespace.uid"] == "" {
		return "", nil // Node/cluster-only observations do not belong to a Namespace.
	}
	if values["k8s.namespace.uid"] == "" {
		if len(objects) == 0 {
			// A name-only observation remains Cluster-scoped. Never infer a
			// Namespace generation from a mutable name or grant it row access.
			return "", errNamespaceAttributionUnverified
		}
		resolver, ok := server.Control.(IngestMetricObjectNamespaceResolver)
		if !ok {
			return "", errIngestRejected
		}
		return resolver.ResolveMetricObjectNamespace(ctx, identity, source, values["k8s.namespace.name"], objects)
	}
	resolver, ok := server.Control.(IngestMetricNamespaceResolver)
	if !ok {
		return "", errIngestRejected
	}
	return resolver.ResolveMetricNamespace(ctx, identity, source, values["k8s.namespace.name"], values["k8s.namespace.uid"])
}
