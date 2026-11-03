package agent

import (
	"encoding/json"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Only facts checked against the exact current tool messages enter the dispatch
// receipt. It stores no original data and cannot be authored by the model.
func modelInlineQueryReceipts(projection ContextProjection, requestHash string) []byte {
	receipts := []map[string]any{}
	for _, part := range projection.Parts {
		if part.Kind != "typed_checkpoint" {
			continue
		}
		raw, err := json.Marshal(part.Content)
		if err != nil {
			continue
		}
		var facts struct {
			Evidence struct {
				Items []queryEvidenceFile `json:"items"`
			} `json:"query_evidence"`
		}
		if json.Unmarshal(raw, &facts) != nil {
			continue
		}
		for _, file := range facts.Evidence.Items {
			matched := false
			for _, message := range projection.Messages {
				var out struct {
					CallID   string `json:"tool_call_id"`
					Hash     string `json:"result_hash"`
					Bytes    int32  `json:"original_bytes"`
					Complete bool   `json:"inline_original_complete"`
					Partial  bool   `json:"partial"`
				}
				if message.Role == "tool" && json.Unmarshal([]byte(message.Content), &out) == nil && out.Complete && !out.Partial && out.CallID == file.CallID && out.Hash == file.SHA256 && out.Bytes == file.Bytes {
					matched = true
					break
				}
			}
			if file.Available && file.InlineOriginalInContext && matched && file.Bytes <= toolruntime.MaxInlineQueryOriginalBytes && len(receipts) < 64 {
				receipts = append(receipts, map[string]any{"query_call_id": file.CallID, "original_sha256": file.SHA256, "original_bytes": file.Bytes, "provider_request_hash": requestHash})
			}
		}
	}
	raw, _ := json.Marshal(receipts)
	return raw
}
