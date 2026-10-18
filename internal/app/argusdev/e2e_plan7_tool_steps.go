package argusdev

// Exercise the documented category discovery and description sequence before
// invoking a native plan, including in deterministic browser/fault fixtures.
func plan7PlanToolSequence(preview p5ToolStep) []p5ToolStep {
	return []p5ToolStep{
		{Tool: "tool.search", Arguments: map[string]any{"category": "workflow"}},
		{Tool: "tool.describe", Arguments: map[string]any{"category": "workflow", "name": "remote_plan.preview"}},
		preview,
	}
}
