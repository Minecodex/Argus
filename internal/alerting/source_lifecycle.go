package alerting

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
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
	for _, previous := range old {
		if !overlappingIDs(row.ResourceIds, []uuid.UUID{previous.ResourceID}) {
			continue
		}
		found := false
		for _, current := range result.Sources {
			if current.ID == previous.ID && current.Generation == previous.Generation {
				found = true
				break
			}
		}
		if !found {
			return "source_changed", nil
		}
	}
	return "", nil
}
