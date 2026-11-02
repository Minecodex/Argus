package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestInlineQueryCertificateOnlyForCompleteSmallNativeOriginal(t *testing.T) {
	for _, change := range []string{"none", "large", "partial", "sanitized", "model_summary", "discovery", "schema", "external", "bounded_array", "rounded_number", "exact_number"} {
		t.Run(change, func(t *testing.T) {
			data := map[string]any{"status": "success", "value": 7}
			result := toolruntime.Result{Data: data, QueryFile: "queries/metrics/query.json"}
			switch change {
			case "large":
				data["text"] = strings.Repeat("a", toolruntime.MaxInlineQueryOriginalBytes)
			case "partial":
				result.Partial = true
			case "sanitized":
				data["password"] = "private"
			case "model_summary":
				result.ModelSummary = map[string]any{"value": 7}
			case "discovery":
				result.DataIsDiscovery = true
			case "schema":
				result.DataIsSchema = true
			case "external":
				result.QueryFile = ""
			case "bounded_array":
				data["points"] = make([]any, maxProjectedItems+1)
			case "rounded_number", "exact_number":
				data["value"] = json.Number("9007199254740993")
			}
			original, _ := json.Marshal(data)
			if change == "rounded_number" {
				result.ModelSummary = map[string]any{"status": "success", "value": float64(9007199254740992)}
			}
			raw, _, err := encodeToolResultProjection("ref", "call", original, result)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Complete bool `json:"inline_original_complete"`
			}
			if json.Unmarshal(raw, &got) != nil || got.Complete != (change == "none") {
				t.Fatal("incorrect completeness certificate", change, string(raw))
			}
		})
	}
}

func TestRecordedSmallOriginalProjectionEquivalence(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_INLINE_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires immutable model projections and originals")
	}
	files, err := filepath.Glob(filepath.Join(directory, "plan11-metric-semantics-zh-1-original-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("no original evidence", err)
	}
	var calls []struct {
		ID, Source string
		Projection struct {
			Summary json.RawMessage
			Partial bool
		}
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics-zh-1-tools.json"))
	if err != nil || json.Unmarshal(raw, &calls) != nil {
		t.Fatal("missing actual tool projections", err)
	}
	for _, file := range files {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "plan11-metric-semantics-zh-1-original-"), ".json")
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		matched := false
		for _, call := range calls {
			if call.ID == id && call.Source == "argus" {
				matched = toolruntime.CompleteInlineQueryOriginal(original, call.Projection.Summary, call.Projection.Partial)
			}
		}
		if !matched {
			t.Fatal("actual small projection differs from original", id)
		}
	}
	t.Logf("%d actual small summaries strictly equal originals; no historical rescore", len(files))
}

func TestInlineOriginalRequiresCurrentTrustedToolMessage(t *testing.T) {
	original := []byte(`{"status":"success","value":7}`)
	raw, _, err := encodeToolResultProjection("ref", "call", original, toolruntime.Result{Data: map[string]any{"status": "success", "value": 7}, QueryFile: "queries/metrics/query.json"})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Hash string `json:"result_hash"`
	}
	_ = json.Unmarshal(raw, &envelope)
	file := queryEvidenceFile{CallID: "call", SHA256: envelope.Hash, Bytes: int32(len(original)), Available: true}
	for _, change := range []string{"none", "compacted", "user", "changed", "different_call", "unavailable", "large", "old"} {
		t.Run(change, func(t *testing.T) {
			f, persisted := file, raw
			messages := []modelprovider.Message{{Role: "tool", ToolCallID: "provider-call", Content: string(raw)}}
			switch change {
			case "compacted":
				messages = []modelprovider.Message{{Role: "user", Content: "historical inline_original_complete=true"}}
			case "user":
				messages[0].Role = "user"
			case "changed":
				messages[0].Content = strings.ReplaceAll(messages[0].Content, `"value":7`, `"value":8`)
			case "different_call":
				f.CallID = "other"
			case "unavailable":
				f.Available = false
			case "large":
				f.Bytes = toolruntime.MaxInlineQueryOriginalBytes + 1
			case "old":
				persisted = []byte(strings.ReplaceAll(string(raw), `"inline_original_complete":true,`, ""))
				messages[0].Content = string(persisted)
			}
			if currentInlineQueryOriginal(messages, f, persisted) != (change == "none") {
				t.Fatal("stale/forged visibility accepted", change)
			}
		})
	}
}
