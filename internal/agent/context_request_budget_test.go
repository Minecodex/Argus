package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func requestBudgetFixture(t *testing.T, tail []modelprovider.Message, facts map[string]any) (ContextProjection, func([]modelprovider.Message) (int, error)) {
	t.Helper()
	projection, err := AssembleContext(ContextInput{ContextWindow: 131072, MaxOutput: 1024,
		System: modelprovider.Message{Role: "system", Content: "Use current authority and exact data."}, Checkpoint: facts,
		RecentTail: tail, ToolCatalog: []modelprovider.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	measure := func(messages []modelprovider.Message) (int, error) {
		return (modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions}).RequestBytes(inferenceRequest("fixture", 1024, messages, nil, "", ""))
	}
	return projection, measure
}

func TestRequestBudgetReferencesFullResultWithoutChangingOriginalOrCertifyingInline(t *testing.T) {
	id := "source-call"
	data := map[string]any{"stdout": strings.Repeat("exact-full-original-data", 700), "exit_code": 1, "partial": false}
	full, _ := json.Marshal(data)
	encoded, _, err := encodeToolResultProjection("result_"+id, id, full, toolruntime.Result{Data: data, QueryFile: "queries/exact/full.json"})
	if err != nil {
		t.Fatal(err)
	}
	tail := []modelprovider.Message{{Role: "user", Content: "Keep this exact constraint and /workspace/exact-full-id.csv"},
		{Role: "assistant", ToolCalls: []modelprovider.ToolCall{{ID: id, Name: "bash", Arguments: `{"command":"already executed"}`}}},
		{Role: "tool", ToolCallID: id, Content: string(encoded)}}
	facts := map[string]any{"query_evidence": map[string]any{"items": []map[string]any{{"query_call_id": id, "inline_original_in_context": true, "available": true}}}}
	original, measure := requestBudgetFixture(t, tail, facts)
	before, _ := json.Marshal(original.Parts)
	size, _ := measure(original.Messages)
	original.HardLimit = size - 3000
	view, err := fitContextRequest(original, nil, nil, measure)
	if err != nil || view.NeedsHardCompaction() {
		t.Fatal("reducible request did not fit", err, view.EstimatedTokens)
	}
	after, _ := json.Marshal(original.Parts)
	if string(before) != string(after) || original.Messages[len(original.Messages)-1].Content != string(encoded) {
		t.Fatal("budget projection mutated original evidence")
	}
	var result map[string]any
	if json.Unmarshal([]byte(view.Messages[len(view.Messages)-1].Content), &result) != nil {
		t.Fatal("invalid projected JSON")
	}
	summary := result["summary"].(map[string]any)
	if result["result_ref"] != "result_"+id || result["partial"] != true || result["inline_original_complete"] != false || summary["exit_code"] != float64(1) || summary["stdout"] != nil {
		t.Fatal("result identity, partial state or nonzero execution outcome lost", result)
	}
	if strings.Contains(view.Messages[1].Content, `"inline_original_in_context":true`) || string(modelInlineQueryReceipts(view, "request")) != "[]" {
		t.Fatal("omitted original was certified inline")
	}
	if !strings.Contains(view.Messages[2].Content, "/workspace/exact-full-id.csv") || view.Messages[3].ToolCalls[0].ID != id || view.Messages[4].ToolCallID != id {
		t.Fatal("user constraints or tool message pairing changed")
	}
	if view.Hash == original.Hash {
		t.Fatal("actual model view retained the unmodified projection hash")
	}
}

func TestRequestBudgetReleasesOnlyCompleteOptionalHistoricalSchemaPair(t *testing.T) {
	contract := func(id string) []modelprovider.Message {
		data := map[string]any{"input_schema": map[string]any{"required": []string{"api_key"}}, "examples": strings.Repeat("schema-example", 700)}
		full, _ := json.Marshal(data)
		body, _, err := encodeToolResultProjection("result_"+id, id, full, toolruntime.Result{Data: data, DataIsSchema: true})
		if err != nil {
			t.Fatal(err)
		}
		return []modelprovider.Message{{Role: "assistant", ToolCalls: []modelprovider.ToolCall{{ID: id, Name: "tool.describe", Arguments: `{}`}}}, {Role: "tool", ToolCallID: id, Content: string(body)}}
	}
	tail := []modelprovider.Message{{Role: "user", Content: "current goal"}}
	tail = append(tail, contract("old-schema")...)
	tail = append(tail, contract("latest-schema")...)
	projection, measure := requestBudgetFixture(t, tail, map[string]any{})
	size, _ := measure(projection.Messages)
	projection.HardLimit = size - 3000
	view, err := fitContextRequest(projection, []string{"old-schema", "latest-schema"}, nil, measure)
	if err != nil || view.NeedsHardCompaction() || len(view.Messages) != 5 {
		t.Fatal("optional pair was not released as a whole", err, len(view.Messages))
	}
	if view.Messages[4].ToolCallID != "latest-schema" || !strings.Contains(view.Messages[4].Content, `api_key`) || strings.Contains(view.Messages[1].Content, `latest-schema`) {
		t.Fatal("required fields corrupted or too many contracts released")
	}
	// A non-optional contract is never turned into an invented partial schema.
	unreduced, err := fitContextRequest(projection, nil, nil, measure)
	if err != nil || !unreduced.NeedsHardCompaction() {
		t.Fatal("fixed declaration/contract capacity guard was bypassed", err)
	}
}

func TestReferenceProjectionPreservesValidationAndActionFeedback(t *testing.T) {
	for _, field := range []string{"error_code", "action_ref", "control"} {
		data := map[string]any{field: "exact-value", "details": strings.Repeat("important", 1000)}
		full, _ := json.Marshal(data)
		body, _, err := encodeToolResultProjection("result_call", "call", full, toolruntime.Result{Data: data})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, ok := referenceOnlyResult(modelprovider.Message{Role: "tool", ToolCallID: "call", Content: string(body)}); ok {
			t.Fatal("critical feedback was replaced", field)
		}
	}
}
