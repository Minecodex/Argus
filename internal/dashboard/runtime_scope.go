package dashboard

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type ResourceScope struct {
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"`
}
type ResolvedSource struct {
	ID                  uuid.UUID  `json:"id"`
	Revision            int64      `json:"revision"`
	ResourceID          uuid.UUID  `json:"resource_id"`
	Generation          uuid.UUID  `json:"generation"`
	Type                string     `json:"type"`
	CapabilityVersion   string     `json:"capability_version"`
	CurrentInstallation *bool      `json:"current_installation,omitempty"`
	RegisteredAt        *time.Time `json:"registered_at,omitempty"`
}

func (source ResolvedSource) Key() string {
	return source.ID.String() + ":" + strconv.FormatInt(source.Revision, 10)
}

var sourceSignals = map[string][]string{
	"otlp": {"metrics", "logs", "traces"}, "hostmetrics": {"metrics"}, "prometheus": {"metrics"}, "kubeletstats": {"metrics"}, "k8s_cluster": {"metrics"},
	"filelog": {"logs"}, "journald": {"logs"}, "windowseventlog": {"logs"},
	"skywalking": {"traces"}, "jaeger": {"traces"},
}

func resolveResources(ctx context.Context, q *db.Queries, actor Actor, requested []uuid.UUID) ([]ResourceScope, error) {
	if len(requested) > 1000 {
		return nil, ErrInvalid
	}
	result := []ResourceScope{}
	for _, kind := range []string{"host", "kubernetes_cluster"} {
		ids, err := authorizedIDs(ctx, q, actor, kind)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if len(requested) > 0 && !slices.Contains(requested, id) {
				continue
			}
			if kind == "host" {
				item, e := q.GetHost(ctx, db.GetHostParams{ID: id, EnterpriseID: actor.EnterpriseID})
				if e != nil || item.Status != "active" {
					continue
				}
			} else {
				item, e := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: id, EnterpriseID: actor.EnterpriseID})
				if e != nil || item.Status != "active" {
					continue
				}
			}
			result = append(result, ResourceScope{ID: id, Type: kind})
		}
	}
	for _, id := range requested {
		if !slices.ContainsFunc(result, func(resource ResourceScope) bool { return resource.ID == id }) {
			return nil, ErrDenied
		}
	}
	if len(result) > 1000 {
		return nil, ErrInvalid
	}
	return result, nil
}

func resolveSources(ctx context.Context, q *db.Queries, actor Actor, binding SourceBinding, signal string, resources []uuid.UUID) ([]ResolvedSource, error) {
	if binding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[binding.SourceType], signal) {
		return nil, ErrInvalid
	}
	if len(resources) == 0 {
		return []ResolvedSource{}, nil
	}
	rows, err := q.ListTelemetrySources(ctx, db.ListTelemetrySourcesParams{EnterpriseID: actor.EnterpriseID, ResourceIds: resources, SourceType: binding.SourceType, Signal: signal})
	if err != nil {
		return nil, err
	}
	if len(rows) > 10000 {
		return nil, ErrInvalid
	}
	result := []ResolvedSource{}
	for _, row := range rows {
		if row.CapabilityVersion != binding.CapabilityVersion {
			return nil, ErrInvalid
		}
		current, registered := row.CurrentInstallation, row.CreatedAt.Time
		result = append(result, ResolvedSource{ID: row.ID, Revision: row.ConfigRevision, ResourceID: row.ResourceID, Generation: row.Generation, Type: row.SourceType, CapabilityVersion: row.CapabilityVersion, CurrentInstallation: &current, RegisteredAt: &registered})
	}
	return result, nil
}
