package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Tool evidence is a structured document. The line-oriented log redactor can
// discard the entire JSON array when a public Schema mentions a sensitive key.
// Reuse the field-level diagnostic policy and preserve every call identity.
func writePlan11ToolEvidence(path, raw string) error {
	if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
		return errors.New("tool evidence must be a non-empty JSON array document")
	}
	var calls []map[string]any
	if err := json.Unmarshal([]byte(raw), &calls); err != nil {
		return err
	}
	ids := make([]string, 0, len(calls))
	seen := map[string]bool{}
	for _, call := range calls {
		id, _ := call["id"].(string)
		tool, _ := call["tool"].(string)
		source, _ := call["source"].(string)
		if uuid.Validate(id) != nil || seen[id] || tool == "" || source == "" {
			return errors.New("tool evidence omitted a call identity, origin or tool name")
		}
		ids, seen[id] = append(ids, id), true
	}
	encoded, err := redactedJSONDocument(calls)
	if err != nil || !json.Valid(encoded) || len(encoded) == 0 {
		return errors.Join(err, errors.New("structured tool evidence redaction did not produce valid JSON"))
	}
	originalHash := sha256.Sum256([]byte(raw))
	exportedHash := sha256.Sum256(encoded)
	receipt, err := json.MarshalIndent(map[string]any{
		"schema_version": "argus.plan11_tool_evidence_redaction/v1", "records": len(calls), "call_ids": ids,
		"source_query_bytes": len(raw), "source_query_sha256": hex.EncodeToString(originalHash[:]),
		"exported_bytes": len(encoded), "exported_sha256": hex.EncodeToString(exportedHash[:]),
		"redaction_policy":          "existing structured diagnostic sensitive-field values; public Schema sensitive-key entries may also be redacted",
		"unredacted_original_saved": false, "projection_hashes": "persisted hashes refer to source projections, not redacted exported bytes",
	}, "", "  ")
	if err != nil {
		return err
	}
	if err = writePrivate(path, encoded); err != nil {
		return err
	}
	return writePrivate(path+".redaction.json", receipt)
}
