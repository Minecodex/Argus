package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSignalOperationalLogUsesContainedActualHTTPMarker(t *testing.T) {
	resource := uuid.New()
	marker := "plan11-operational-log-owned"
	response := map[string]any{"schema_version": "argus.kql_result/v1", "result_type": "log_entries", "meta": map[string]any{"engine": "argus-kql", "engine_version": "v2", "plan_hash": strings.Repeat("a", 64), "partial": false, "scanned_rows": 1, "returned_rows": 1, "scanned_bytes": float64(100), "loaded_samples": 0, "elapsed_ms": float64(1)}, "data": []any{map[string]any{"resource_id": resource.String(), "body": "10.0.0.1 - - [actual-time] GET /" + marker + " HTTP/1.1 404"}}}
	if assertKQLResponse(response, marker, 1) == nil {
		t.Fatal("old literal-body defect was not reproduced")
	}
	if err := plan8ScopedLogMatches(response, resource, marker); err != nil {
		t.Fatal("actual access-log message rejected", err)
	}
	if plan8ScopedLogMatches(response, uuid.New(), marker) == nil {
		t.Fatal("foreign observation owner accepted")
	}
	response["meta"].(map[string]any)["partial"] = true
	if plan8ScopedLogMatches(response, resource, marker) == nil {
		t.Fatal("partial response accepted")
	}
}

func TestSignalOperationalLogRecordedActualCRIBody(t *testing.T) {
	root := os.Getenv("ARGUS_PLAN11_OPERATIONAL_REPLAY_DIR")
	if root == "" {
		t.Skip("explicit immutable realCRI response required")
	}
	raw, err := os.ReadFile(filepath.Join(root, "plan8-deployed-kubernetes-collection.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct{ Logs map[string]any }
	if json.Unmarshal(raw, &proof) != nil {
		t.Fatal("invalid actualCRI artifact")
	}
	rows, _ := proof.Logs["data"].([]any)
	if len(rows) != 1 {
		t.Fatal("actualCRI record unavailable")
	}
	row := rows[0].(map[string]any)
	body, _ := row["body"].(string)
	marker := regexp.MustCompile(`plan8-workload-log-[0-9a-f-]+`).FindString(body)
	resource, err := uuid.Parse(row["resource_id"].(string))
	if err != nil || marker == "" {
		t.Fatal("actualCRI identity/marker missing")
	}
	if assertKQLResponse(proof.Logs, marker, 1) == nil {
		t.Fatal("old checker did not reject actual complete body")
	}
	if err = plan8ScopedLogMatches(proof.Logs, resource, marker); err != nil {
		t.Fatal("actual completeCRI body not accepted", err)
	}
}
