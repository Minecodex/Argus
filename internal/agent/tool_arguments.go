package agent

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// JSONB may hold a non-object value. Invalid JSON is retained as a JSON string,
// never repaired into executable arguments. The exact protocol string also
// remains in the paired assistant event. This is safe across worker restarts:
// executePersistedTool checks the object shape before any dispatch.
func storedToolInput(arguments string) []byte {
	var value any
	if json.Unmarshal([]byte(arguments), &value) != nil {
		value = arguments
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func invalidToolArgumentObject() toolruntime.Error {
	return toolruntime.Error{Kind: "TOOL_INPUT_INVALID", Message: "Tool arguments must be one valid JSON object. Correct the JSON and retry; no tool operation was executed.", Details: map[string]any{"schema_validation": true, "issues": []map[string]any{{"path": "/arguments", "constraint": "json_object", "expected": []string{"object"}}}}}
}
