package argusdev

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestMetricInlineEvidenceRequiresActualNativeProjectionAndOriginal(t *testing.T) {
	original := []byte(`{"status":"success","value":7}`)
	hash := fmt.Sprintf("%x", sha256.Sum256(original))
	for _, change := range []string{"none", "external", "old", "partial", "changed", "wrong_hash", "failed", "wrong_tool"} {
		t.Run(change, func(t *testing.T) {
			projection := map[string]any{"schema_version": "argus.tool_result_projection/v1", "tool_call_id": "call", "result_hash": hash, "original_bytes": len(original), "inline_original_complete": true, "partial": false, "summary": json.RawMessage(original)}
			call := map[string]any{"id": "call", "tool": "tool.invoke", "source": "argus", "status": "succeeded", "input": map[string]any{"category": "metric", "name": "query"}, "projection": projection}
			switch change {
			case "external":
				call["source"] = "external_mcp"
			case "old":
				delete(projection, "inline_original_complete")
			case "partial":
				projection["partial"] = true
			case "changed":
				projection["summary"] = map[string]any{"status": "success", "value": 8}
			case "wrong_hash":
				projection["result_hash"] = strings.Repeat("0", 64)
			case "failed":
				call["status"] = "failed"
			case "wrong_tool":
				call["input"] = map[string]any{"category": "observe", "name": "catalog.query"}
			}
			raw, _ := json.Marshal([]any{call})
			if plan11MetricInlineOriginal(string(raw), "call", hash, original) != (change == "none") {
				t.Fatal("untrusted inline evidence accepted", change)
			}
		})
	}
	if toolruntime.CompleteInlineQueryOriginal([]byte(`{"n":9007199254740993}`), map[string]any{"n": float64(9007199254740992)}, false) {
		t.Fatal("float rounding certified")
	}
}
