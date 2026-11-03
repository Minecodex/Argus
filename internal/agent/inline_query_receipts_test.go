package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestInlineQueryReceiptsBindActualCurrentToolMessageAndRequest(t *testing.T) {
	original := []byte(`{"value":7}`)
	raw, _, err := encodeToolResultProjection("ref", "query", original, toolruntime.Result{Data: map[string]any{"value": 7}, QueryFile: "queries/metrics/query.json"})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Hash string `json:"result_hash"`
	}
	_ = json.Unmarshal(raw, &out)
	file := queryEvidenceFile{CallID: "query", SHA256: out.Hash, Bytes: int32(len(original)), Available: true, InlineOriginalInContext: true}
	for _, change := range []string{"none", "missing", "user", "unavailable", "not_current"} {
		t.Run(change, func(t *testing.T) {
			f := file
			messages := []modelprovider.Message{{Role: "tool", Content: string(raw)}}
			switch change {
			case "missing":
				messages = nil
			case "user":
				messages[0].Role = "user"
			case "unavailable":
				f.Available = false
			case "not_current":
				f.InlineOriginalInContext = false
			}
			p := ContextProjection{Messages: messages, Parts: []ContextPart{{Kind: "typed_checkpoint", Content: map[string]any{"query_evidence": map[string]any{"items": []queryEvidenceFile{f}}}}}}
			var receipts []map[string]any
			encoded := modelInlineQueryReceipts(p, strings.Repeat("a", 64))
			if json.Unmarshal(encoded, &receipts) != nil || (len(receipts) == 1) != (change == "none") {
				t.Fatal("forged receipt", change, string(encoded))
			}
			if len(receipts) > 0 && (receipts[0]["provider_request_hash"] != strings.Repeat("a", 64) || strings.Contains(string(encoded), "value")) {
				t.Fatal("receipt leaked data or lost request binding")
			}
		})
	}
}

func TestContextEventHistoryPreservesOriginalJSONNumbers(t *testing.T) {
	raw := []byte(`{"authorization_scope":"scope","tool_call_id":"call","projection":{"summary":{"n":9007199254740993}}}`)
	visible, err := contextEventPayload("tool_call_result", raw, "scope")
	encoded, _ := json.Marshal(visible)
	if err != nil || !strings.Contains(string(encoded), "9007199254740993") || strings.Contains(string(encoded), "9007199254740992") {
		t.Fatal("history rounded original data", err, string(encoded))
	}
}
