package dashboard

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"time"
)

// CreationCatalog exposes only object-authorized names and registered source
// capabilities. Receiving data is independently checked through Catalog.
func (service Service) CreationCatalog(ctx context.Context, actor Actor, offset, limit int) (map[string]any, error) {
	q := service.Store.Queries
	if err := authorize(ctx, q, actor, "telemetry.dashboard.read"); err != nil {
		return nil, err
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	// Directory pagination must not require admitting the entire enterprise as
	// one execution scope. The selected page is bounded before loading metadata.
	resources := []ResourceScope{}
	for _, kind := range []string{"host", "kubernetes_cluster"} {
		authorized, err := authorizedIDs(ctx, q, actor, kind)
		if err != nil {
			return nil, err
		}
		for _, id := range authorized {
			resources = append(resources, ResourceScope{ID: id, Type: kind})
		}
	}
	slices.SortFunc(resources, func(a, b ResourceScope) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	start := min(offset, len(resources))
	end := min(start+limit, len(resources))
	ids := []uuid.UUID{}
	items := []map[string]any{}
	for _, r := range resources[start:end] {
		name := ""
		if r.Type == "host" {
			row, e := q.GetHost(ctx, db.GetHostParams{ID: r.ID, EnterpriseID: actor.EnterpriseID})
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return nil, e
			}
			if row.Status != "active" {
				continue
			}
			name = row.Name
		} else {
			row, e := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: r.ID, EnterpriseID: actor.EnterpriseID})
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return nil, e
			}
			if row.Status != "active" {
				continue
			}
			name = row.Name
		}
		ids = append(ids, r.ID)
		items = append(items, map[string]any{"id": r.ID, "type": r.Type, "name": name})
	}
	capabilities := []map[string]any{}
	types := []string{}
	for kind := range sourceSignals {
		types = append(types, kind)
	}
	slices.Sort(types)
	if len(ids) > 0 {
		for _, kind := range types {
			for _, signal := range sourceSignals[kind] {
				sources, e := resolveSources(ctx, q, actor, SourceBinding{SourceType: kind, CapabilityVersion: "v1"}, signal, ids)
				if e != nil {
					return nil, e
				}
				matched := []uuid.UUID{}
				for _, s := range sources {
					if !slices.Contains(matched, s.ResourceID) {
						matched = append(matched, s.ResourceID)
					}
				}
				if len(matched) > 0 {
					capabilities = append(capabilities, map[string]any{"source_type": kind, "signal": signal, "capability_version": "v1", "resource_ids": matched, "data_state": "not_sampled"})
				}
			}
		}
	}
	enterprise, err := q.GetEnterprise(ctx, actor.EnterpriseID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	from := now.Add(-time.Duration(EmptySpec().DefaultTimeRange.Seconds) * time.Second)
	return map[string]any{"resources": items, "source_capabilities": capabilities, "has_more": end < len(resources), "next_offset": end,
		"server_time": now, "timezone": enterprise.Timezone, "suggested_catalog_range": map[string]any{"from": from, "to": now}}, nil
}
