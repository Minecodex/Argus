package toolruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputFeedbackNamesMissingFieldsWithoutEchoingValues(t *testing.T) {
	schema, err := CompileInputSchema(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"title"}, "properties": map[string]any{"title": map[string]any{"type": "string"}, "count": map[string]any{"type": "integer"}}})
	if err != nil {
		t.Fatal(err)
	}
	validation := schema.Validate(map[string]any{"count": "secret-input-value", "unexpected": "secret-extra-value"})
	if validation == nil {
		t.Fatal("invalid fixture accepted")
	}
	feedback := InputValidationError(validation, "/arguments")
	raw, _ := json.Marshal(feedback.Details)
	text := string(raw)
	for _, expected := range []string{"/arguments/title", "required", "/arguments/count", "type", "integer", "/arguments/unexpected", "additionalProperties"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing correction detail %s: %s", expected, text)
		}
	}
	if strings.Contains(text, "secret-input-value") || strings.Contains(text, "secret-extra-value") {
		t.Fatal("rejected values leaked")
	}
}
