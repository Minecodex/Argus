package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordedR60ActualSandboxReadFacts(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R60_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires immutable R60 actual sandbox tool evidence and originals")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics-zh-1-tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		Tool, Source, Status string
		Input                json.RawMessage
		Projection           json.RawMessage
	}
	if json.Unmarshal(raw, &calls) != nil {
		t.Fatal("invalid actual recorded trace")
	}
	verified := 0
	for _, call := range calls {
		if call.Tool != "read" || call.Source != "sandbox" || call.Status != "succeeded" {
			continue
		}
		var input struct{ Path string }
		if json.Unmarshal(call.Input, &input) != nil {
			t.Fatal("missing actual read path")
		}
		id := strings.TrimSuffix(filepath.Base(input.Path), ".json")
		body, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics-zh-1-original-"+id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(body)
		if !completeQueryFileRead(call.Input, call.Projection, queryEvidenceFile{Path: input.Path, SHA256: hex.EncodeToString(hash[:]), Bytes: int32(len(body)), Available: true}) {
			t.Fatal("actual sandbox full original read was not verified", id)
		}
		verified++
	}
	if verified != 5 {
		t.Fatal("recorded actual sandbox reads missing", verified)
	}
	t.Log("Five actual R60 sandbox reads match downloaded original hashes/bytes; provenance is sandbox, not native argus; no model calls/rescore")
}
