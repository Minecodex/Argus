package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStructuredToolEvidenceKeepsEveryCallAndRedactsFieldValues(t *testing.T) {
	first, second := uuid.NewString(), uuid.NewString()
	raw := `[{"id":"` + first + `","tool":"tool.describe","source":"argus","input":{"category":"host","name":"get"},"projection":{"summary":{"description":"Public authorization/password Schema metadata","properties":{"password":{"type":"string"}}}}},{"id":"` + second + `","tool":"tool.invoke","source":"argus","input":{"resource_id":"explicit-resource","password":"fixture-sensitive-value"},"projection":{"summary":{"token":"fixture-token-value","resource_id":"explicit-resource"}}}]`
	if len(redactDiagnostic([]byte(raw))) != 0 {
		t.Fatal("fixture no longer reproduces the original whole-line evidence loss")
	}
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := writePlan11ToolEvidence(path, raw); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil || !json.Valid(encoded) || strings.Contains(string(encoded), "fixture-sensitive-value") || strings.Contains(string(encoded), "fixture-token-value") {
		t.Fatal("export is invalid or leaked sensitive field values", err)
	}
	var calls []map[string]any
	if json.Unmarshal(encoded, &calls) != nil || len(calls) != 2 || calls[0]["id"] != first || calls[1]["id"] != second || !strings.Contains(string(encoded), "explicit-resource") {
		t.Fatal("redaction removed a record or non-sensitive identity")
	}
	var receipt struct {
		Records         int      `json:"records"`
		IDs             []string `json:"call_ids"`
		SourceHash      string   `json:"source_query_sha256"`
		ExportHash      string   `json:"exported_sha256"`
		UnredactedSaved bool     `json:"unredacted_original_saved"`
	}
	data, err := os.ReadFile(path + ".redaction.json")
	if err != nil || json.Unmarshal(data, &receipt) != nil || receipt.Records != 2 || len(receipt.IDs) != 2 || len(receipt.SourceHash) != 64 || len(receipt.ExportHash) != 64 || receipt.SourceHash == receipt.ExportHash || receipt.UnredactedSaved {
		t.Fatal("export lacks accurate redaction provenance", err)
	}
}

func TestStructuredToolEvidenceRejectsEmptyInvalidOrIdentitylessDocuments(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", "[", `[{"id":"missing","tool":"read","source":"sandbox"}]`} {
		path := filepath.Join(t.TempDir(), "tools.json")
		if writePlan11ToolEvidence(path, raw) == nil {
			t.Fatal("invalid evidence export appeared successful", raw)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid export wrote an evidence file")
		}
	}
	if err := writePlan11ToolEvidence(filepath.Join(t.TempDir(), "empty-trace.json"), "[]"); err != nil {
		t.Fatal("an actual zero-tool Run must retain a valid empty array", err)
	}
}
