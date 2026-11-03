package argusdev

// Workspace bash can complete as a tool while the process exits nonzero. Keep
// process facts alongside the complete input; tool status alone is insufficient
// evidence that a file was read or statistics were computed successfully.
func plan11ModelToolTraceSQL(run string) string {
	return `SELECT coalesce(json_agg(json_build_object(
 'id',t.id,'tool',t.tool_id,'input',t.input,'status',t.status,'error_code',t.error_code,
 'exit_code',r.projection->'summary'->'exit_code',
 'partial',r.projection->'summary'->'partial'
 ) ORDER BY t.created_at,t.call_sequence),'[]')
 FROM tool_calls t LEFT JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id
 WHERE t.run_id='` + run + `'`
}

type plan11ProcessOutcome struct {
	ExitCode *int  `json:"exit_code"`
	Partial  *bool `json:"partial"`
}

func (p plan11ProcessOutcome) success() bool {
	return p.ExitCode != nil && *p.ExitCode == 0 && p.Partial != nil && !*p.Partial
}
