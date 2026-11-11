package alerting

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
)

func missingEpisodeClosure(ctx context.Context, q *db.Queries, row db.AlertInstance, result Evaluation) (string, error) {
	exists, err := q.AlertResourcesStillExist(ctx, db.AlertResourcesStillExistParams{EnterpriseID: row.EnterpriseID, ResourceIds: row.ResourceIds})
	if err != nil {
		return "", err
	}
	if !exists {
		return "resource_deleted", nil
	}
	if !result.SourcesCaptured || result.Evaluator.SubjectID == uuid.Nil || !containsAll(result.ResourceIDs, row.ResourceIds) {
		return "", nil
	}
	var state InstanceState
	if json.Unmarshal(row.State, &state) != nil {
		return "", ErrInvalid
	}
	var old []db.AlertMetricSourcesRow
	if len(state.SourceSnapshot) == 0 {
		return "", nil
	}
	if json.Unmarshal(state.SourceSnapshot, &old) != nil {
		return "", ErrInvalid
	}
	previousScope := map[uuid.UUID]uuid.UUID{}
	participates := func(source db.AlertMetricSourcesRow) bool {
		return slices.Contains(row.ResourceIds, source.ResourceID) &&
			(len(row.SourceIds) == 0 || slices.Contains(row.SourceIds, source.ID))
	}
	for _, source := range old {
		if participates(source) {
			previousScope[source.ID] = source.Generation
		}
	}
	for _, source := range result.Sources {
		if !participates(source) {
			continue
		}
		if generation, found := previousScope[source.ID]; !found || generation != source.Generation {
			return "source_changed", nil
		}
		delete(previousScope, source.ID)
	}
	if len(previousScope) > 0 {
		return "source_changed", nil
	}
	return "", nil
}
