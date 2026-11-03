package toolruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
)

// MaxInlineQueryOriginalBytes bounds complete originals eligible for direct
// analysis. The immutable artifact and automatic Workspace delivery still apply.
const MaxInlineQueryOriginalBytes = 32 << 10

// CompleteInlineQueryOriginal compares the original with the actual sanitized
// model summary, without float64 rounding. It does not grant trust to a caller;
// the Harness must separately establish native-query identity and current scope.
func CompleteInlineQueryOriginal(original []byte, summary any, partial bool) bool {
	if partial || len(original) == 0 || len(original) > MaxInlineQueryOriginalBytes {
		return false
	}
	visible, err := json.Marshal(summary)
	if err != nil {
		return false
	}
	a, ok := canonicalQueryJSON(original)
	if !ok {
		return false
	}
	b, ok := canonicalQueryJSON(visible)
	return ok && bytes.Equal(a, b)
}

func canonicalQueryJSON(raw []byte) ([]byte, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value map[string]any
	if d.Decode(&value) != nil || value == nil {
		return nil, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	return encoded, err == nil
}

// QueryJSONEqual also tolerates JSONB whitespace/key ordering while preserving
// all number tokens. It never compares via float64.
func QueryJSONEqual(a, b []byte) bool {
	left, ok := canonicalQueryJSON(a)
	if !ok {
		return false
	}
	right, ok := canonicalQueryJSON(b)
	return ok && bytes.Equal(left, right)
}

// InlineQueryProjectionMatches checks a framework projection against separately
// retained original bytes. A model/external tool's self-reported flag is never
// enough: native identity/current authorization must also be verified by caller.
func InlineQueryProjectionMatches(projection, original []byte, callID, originalSHA string) bool {
	var out struct {
		Schema   string          `json:"schema_version"`
		CallID   string          `json:"tool_call_id"`
		Hash     string          `json:"result_hash"`
		Bytes    int             `json:"original_bytes"`
		Partial  bool            `json:"partial"`
		Complete bool            `json:"inline_original_complete"`
		Summary  json.RawMessage `json:"summary"`
	}
	digest := sha256.Sum256(original)
	return json.Unmarshal(projection, &out) == nil && out.Schema == "argus.tool_result_projection/v1" &&
		out.Complete && out.CallID == callID && out.Hash == originalSHA &&
		hex.EncodeToString(digest[:]) == originalSHA && out.Bytes == len(original) &&
		CompleteInlineQueryOriginal(original, out.Summary, out.Partial)
}
