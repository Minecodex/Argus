package argusdev

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Check the actual native projection against independently downloaded immutable
// bytes. External or historical unflagged projections cannot opt in.
func plan11MetricInlineOriginal(trace, callID, expectedSHA string, original []byte) bool {
	var calls []struct {
		ID, Tool, Source, Status string
		Input                    struct{ Category, Name string }
		Projection               json.RawMessage
	}
	if json.Unmarshal([]byte(trace), &calls) != nil {
		return false
	}
	for _, call := range calls {
		if call.ID == callID && call.Tool == "tool.invoke" && call.Source == "argus" && call.Status == "succeeded" && call.Input.Category == "metric" && call.Input.Name == "query" && toolruntime.InlineQueryProjectionMatches(call.Projection, original, callID, expectedSHA) {
			return true
		}
	}
	return false
}

func plan11InlineModelExposureSQL(run, call string) string {
	if uuid.Validate(run) != nil || uuid.Validate(call) != nil {
		return "SELECT 0"
	}
	return `SELECT CASE WHEN EXISTS(SELECT 1 FROM model_calls m JOIN tool_calls t ON t.id='` + call + `' AND t.run_id=m.run_id AND t.enterprise_id=m.enterprise_id
 JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id
 JOIN artifacts a ON a.id=r.artifact_id AND a.enterprise_id=t.enterprise_id
 WHERE m.run_id='` + run + `' AND m.call_kind='inference' AND m.status='succeeded' AND m.dispatched_at IS NOT NULL
 AND m.inline_query_receipts @> jsonb_build_array(jsonb_build_object('query_call_id',t.id::text,'original_sha256',encode(a.content_hash,'hex'),
 'original_bytes',a.byte_size,'provider_request_hash',encode(m.projection_hash,'hex')))) THEN 1 ELSE 0 END`
}
