package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSignalBudgetClassifiesNativeAndDomainErrorsOnly(t *testing.T) {
	for _, code := range []string{"TOOL_QUERY_BUDGET_EXCEEDED", "QUERY_BUDGET_EXCEEDED"} {
		if !plan11SignalBudgetFailure(code) {
			t.Fatal("budget failure lost", code)
		}
	}
	for _, code := range []string{"", "TOOL_EXECUTION_FAILED", "TOOL_INPUT_INVALID", "TOOL_PERMISSION_DENIED"} {
		if plan11SignalBudgetFailure(code) {
			t.Fatal("unrelated failure counted as budget evidence", code)
		}
	}
}

func TestSignalRecordedBudgetStopsWithoutSuccessfulData(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_SIGNAL_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit archived actual signal evidence")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-signal-budget-tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		Tool, Status string
		ErrorCode    string `json:"error_code"`
		Input        struct{ Category, Name string }
	}
	if err = json.Unmarshal(raw, &calls); err != nil {
		t.Fatal(err)
	}
	budget, data := 0, 0
	for _, call := range calls {
		if call.Tool != "tool.invoke" {
			continue
		}
		if plan11SignalBudgetFailure(call.ErrorCode) {
			budget++
		}
		if call.Status == "succeeded" && (call.Input.Category == "metric" || call.Input.Category == "log" || call.Input.Category == "trace") && call.Input.Name == "query" {
			data++
		}
	}
	if budget != 1 || data != 0 {
		t.Fatalf("archived actual budget errors=%d successful data=%d", budget, data)
	}
	t.Log("Actual native budget failure recognized; original machine result retained, no new model call")
}

func TestSignalRecordedAuthorizedWorkspaceImportKeepsTraceBoundary(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_SIGNAL_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit archived actual signal evidence")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-signal-semantics-zh-tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan11SignalTraceAllowed(string(raw)); err != nil {
		t.Fatal("actual authorized import/data queries were classified as a business operation", err)
	}
	t.Log("Actual trace verified without changing original hard-stop or machine results")
}
