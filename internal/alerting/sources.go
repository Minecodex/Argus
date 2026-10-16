package alerting

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"strconv"
)

// Sources are frozen per evaluation, including collector generation and revision.
// Reinstalling a collector or withdrawing authorization invalidates the result.
func metricSources(ctx context.Context, q *db.Queries, enterprise uuid.UUID, ids, requested []uuid.UUID) ([]db.AlertMetricSourcesRow, error) {
	rows, err := q.AlertMetricSources(ctx, db.AlertMetricSourcesParams{EnterpriseID: enterprise, ResourceIds: ids})
	if err != nil {
		return nil, err
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
		return nil, ErrDenied
	}
	return out, nil
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
