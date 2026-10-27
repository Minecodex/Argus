package dashboard

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
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
	snapshot, err := (observationscope.Resolver{Queries: q, Permission: "telemetry.dashboard.read"}).Directory(ctx, actor, observationscope.Selection{Mode: observationscope.AllAuthorized, Signal: "catalog"})
	if err != nil {
		return nil, err
	}
	resources := snapshot.Resources
	start := min(offset, len(resources))
	end := min(start+limit, len(resources))
	ids := []uuid.UUID{}
	refs := []ResourceScope{}
	items := []map[string]any{}
	for _, r := range resources[start:end] {
		ids = append(ids, r.ID)
		refs = append(refs, ResourceScope{Type: r.Kind, ID: r.ID})
		items = append(items, map[string]any{"id": r.ID, "type": r.Kind, "name": r.Name, "environment": r.Environment})
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
				sources, e := resolveSources(ctx, q, actor, SourceBinding{SourceType: kind, CapabilityVersion: "v1"}, signal, refs)
				if e != nil {
					return nil, e
				}
				matched := []uuid.UUID{}
				for _, s := range sources {
					for _, id := range s.ScopeResourceIDs {
						if !slices.Contains(matched, id) {
							matched = append(matched, id)
						}
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
