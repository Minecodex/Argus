package skywalking

import "github.com/kakj-go/Argus/internal/telemetry/queryengine/chscope"

// Every membership subquery must apply the same resource boundary as the
// outer query. A hidden span must not influence even the presence of a trace
// in an authorized list. Scan budgets fail the query instead of silently
// limiting this membership set and producing false negatives.
func (state *executionState) spanFilterScope() (string, []any) {
	// Explicit DateTime64(9) constants retain nanosecond boundaries and avoid
	// the driver's default second-precision DateTime comparison path.
	predicate := "start_time >= fromUnixTimestamp64Nano(?, 'UTC') AND start_time < fromUnixTimestamp64Nano(?, 'UTC') AND resource_id IN (?)"
	args := []any{
		state.request.Start.UnixNano(), state.request.End.UnixNano(), state.request.Scope.ResourceIDs,
	}
	if len(state.request.Scope.SourceKeys) > 0 {
		predicate += " AND " + chscope.ExactPredicate("source_key")
	}
	return predicate, args
}
