package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestQueryReadVerificationRequiresActualCompleteOriginalBytes(t *testing.T) {
	body := []byte(`{"value":7}`)
	hash := sha256.Sum256(body)
	file := queryEvidenceFile{Path: "/workspace/queries/metrics/example.json", SHA256: hex.EncodeToString(hash[:]), Bytes: int32(len(body)), Available: true}
	for _, change := range []string{"none", "terminal_newline", "relative_result_path", "partial", "wrong_path", "wrong_content", "wrong_size", "later_lines", "two_newlines"} {
		t.Run(change, func(t *testing.T) {
			input := map[string]any{"path": file.Path}
			summary := map[string]any{"path": file.Path, "content": string(body), "byte_size": len(body), "start_line": 1, "partial": false}
			switch change {
			case "terminal_newline":
				summary["content"] = string(body) + "\n"
			case "relative_result_path":
				summary["path"] = "queries/metrics/example.json"
			case "partial":
				summary["partial"] = true
			case "wrong_path":
				input["path"] = "/workspace/queries/metrics/other.json"
			case "wrong_content":
				summary["content"] = `{"value":8}`
			case "wrong_size":
				summary["byte_size"] = len(body) + 1
			case "later_lines":
				summary["start_line"] = 2
			case "two_newlines":
				summary["content"] = string(body) + "\n\n"
			}
			in, _ := json.Marshal(input)
			projection, _ := json.Marshal(map[string]any{"partial": false, "summary": summary})
			got := completeQueryFileRead(in, projection, file)
			if got != (change == "none" || change == "terminal_newline" || change == "relative_result_path") {
				t.Fatal("read verification confused invocation/partial/modified bytes with an original full read", change, got)
			}
		})
	}
}
