package alerting

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"strconv"
)

// Sources are frozen per evaluation, including collector generation and revision.
// Reinstalling a collector or withdrawing authorization invalidates the result.
func metricSources(ctx context.Context, q *db.Queries, actor Actor, ids, requested []uuid.UUID, types []string) ([]db.AlertMetricSourcesRow, observationscope.Snapshot, error) {
	snapshot, err := (observationscope.Resolver{Queries: q}).Resolve(ctx, actor, observationscope.Selection{Mode: observationscope.Explicit, IDs: ids, Types: types, Signal: "metrics"})
	if err != nil {
		return nil, snapshot, err
	}
	if len(snapshot.Resources) == 0 {
		return []db.AlertMetricSourcesRow{}, snapshot, nil
	}
	raw, _ := json.Marshal(snapshot.Refs())
	rows, err := q.AlertMetricSources(ctx, db.AlertMetricSourcesParams{EnterpriseID: actor.EnterpriseID, ResourceRefs: raw})
	if err != nil {
		return nil, snapshot, err
	}
	out := []db.AlertMetricSourcesRow{}
	found := []uuid.UUID{}
	for _, row := range rows {
		if len(requested) == 0 || slices.Contains(requested, row.ID) {
			out = append(out, row)
			found = append(found, row.ID)
		}
	}
	if !containsAll(found, requested) {
		return nil, snapshot, ErrDenied
	}
	return out, snapshot, nil
}
func sourceKeys(rows []db.AlertMetricSourcesRow) []string {
	keys := []string{}
	for _, row := range rows {
		keys = append(keys, row.ID.String()+":"+strconv.FormatInt(row.ConfigRevision, 10))
	}
	return keys
}
func sourcesEqual(a, b []db.AlertMetricSourcesRow) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func overlappingIDs(a, b []uuid.UUID) bool {
	for _, id := range a {
		if slices.Contains(b, id) {
			return true
		}
	}
	return false
}
