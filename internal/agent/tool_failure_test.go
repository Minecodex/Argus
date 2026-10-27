package agent

import (
	"encoding/json"
	"errors"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"strings"
	"testing"
)

func TestModelReceivesSafeValidationPathsAndNotInternalErrors(t *testing.T) {
	schema, err := toolruntime.CompileInputSchema(map[string]any{"type": "object", "required": []string{"title"}, "properties": map[string]any{"title": map[string]any{"type": "string"}}})
	if err != nil {
		t.Fatal(err)
	}
	feedback := toolruntime.InputValidationError(schema.Validate(map[string]any{}), "/arguments")
	data := modelToolFailure(feedback, feedback.Kind)
	full, _ := json.Marshal(data)
	projection, _, err := encodeToolResultProjection("result", "call", full, toolruntime.Result{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projection), "/arguments/title") {
		t.Fatalf("model lost correction details: %s", projection)
	}
	for _, cause := range []error{errors.New("private database detail"), toolruntime.Error{Kind: "TOOL_INPUT_INVALID", Message: "private native error", Details: map[string]any{"internal": "secret"}}} {
		raw, _ := json.Marshal(modelToolFailure(cause, "TOOL_INPUT_INVALID"))
		if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "secret") {
			t.Fatal("internal error exposed")
		}
	}
}

func TestModelReceivesOnlyExplicitSafeQueryBudgetFeedback(t *testing.T) {
	const code = "TOOL_QUERY_BUDGET_EXCEEDED"
	feedback := toolruntime.Error{Kind: code, Message: "Narrow the time window.", Details: map[string]any{"query_budget": true}}
	data := modelToolFailure(feedback, code)
	if data["message"] != feedback.Message || data["details"] == nil {
		t.Fatal("known query budget lost its safe recovery advice")
	}
	for _, cause := range []error{errors.New("secret query"), toolruntime.Error{Kind: code, Message: "secret query"}} {
		raw, _ := json.Marshal(modelToolFailure(cause, code))
		if strings.Contains(string(raw), "secret") {
			t.Fatal("unknown budget error exposed internal text")
		}
	}
}
