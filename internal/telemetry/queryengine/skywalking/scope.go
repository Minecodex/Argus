package skywalking

// Every membership subquery must apply the same resource boundary as the
// outer query. A hidden span must not influence even the presence of a trace
// in an authorized list. Scan budgets fail the query instead of silently
// limiting this membership set and producing false negatives.
func (state *executionState) spanFilterScope() (string, []any) {
	predicate := "start_time >= ? AND start_time < ? AND resource_id IN (?)"
	args := []any{
		state.request.Start, state.request.End, state.request.Scope.ResourceIDs,
	}
	if len(state.request.Scope.SourceKeys) > 0 {
		predicate += " AND source_key IN (?)"
		args = append(args, state.request.Scope.SourceKeys)
	}
	return predicate, args
}
