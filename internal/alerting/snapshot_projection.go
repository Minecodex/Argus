package alerting

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// An independent instance grants no access to sibling resources in its policy.
// Private stored facts remain immutable; this projection describes only the
// instance's frozen participation set for API, tools, history and exports.
func projectInstanceSnapshot(state InstanceState, resources, sources []uuid.UUID) (InstanceState, error) {
	if len(state.PolicySnapshot) > 0 {
		policy, err := decodeSpec[PolicySpec](state.PolicySnapshot)
		if err != nil {
			return state, err
		}
		policy.Resources.Mode = "explicit"
		policy.Resources.IDs = append([]uuid.UUID{}, resources...)
		policy.Resources.SourceIDs = append([]uuid.UUID{}, sources...)
		policy.Resources.Matchers = []Matcher{}
		state.PolicySnapshot, err = json.Marshal(policy)
		if err != nil {
			return state, err
		}
	}
	filter := func(raw json.RawMessage) (json.RawMessage, error) {
		if len(raw) == 0 {
			return raw, nil
		}
		var rows []db.AlertMetricSourcesRow
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, err
		}
		rows = slices.DeleteFunc(rows, func(row db.AlertMetricSourcesRow) bool {
			return !slices.Contains(resources, row.ResourceID) || len(sources) > 0 && !slices.Contains(sources, row.ID)
		})
		if rows == nil {
			rows = []db.AlertMetricSourcesRow{}
		}
		return json.Marshal(rows)
	}
	var err error
	state.SourceSnapshot, err = filter(state.SourceSnapshot)
	if err != nil {
		return state, err
	}
	state.LatestSourceSnapshot, err = filter(state.LatestSourceSnapshot)
	return state, err
}
