package argusdev

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPlan8ScopedLogsKeepStructuredBodiesAndRejectWrongScope(t *testing.T) {
	owner := uuid.New()
	marker := "plan8-unique-marker-" + uuid.NewString()
	for _, test := range []struct {
		name, body string
		resource   uuid.UUID
		partial    bool
		rows       int
		want       bool
	}{
		{"literal", marker, owner, false, 1, true},
		{"json message", `{"message":"` + marker + `","level":"info"}`, owner, false, 1, true},
		{"journal", `{"MESSAGE":"` + marker + `","_SYSTEMD_UNIT":"owned.service"}`, owner, false, 1, true},
		{"CRI request", "GET /" + marker + " HTTP/1.1", owner, false, 1, true},
		{"foreign resource", marker, uuid.New(), false, 1, false},
		{"truncated", marker, owner, true, 1, false},
		{"duplicates", marker, owner, false, 2, false},
		{"no samples", marker, owner, false, 0, false},
		{"marker in other field", `{"message":"unrelated","password":"` + marker + `"}`, owner, false, 1, false},
		{"missing marker", "different message", owner, false, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := []any{}
			for i := 0; i < test.rows; i++ {
				rows = append(rows, map[string]any{"resource_id": test.resource.String(), "body": test.body})
			}
			wire := map[string]any{"schema_version": "argus.kql_result/v1", "result_type": "log_entries", "data": rows, "meta": map[string]any{"plan_hash": strings.Repeat("a", 64), "partial": test.partial, "scanned_bytes": float64(1), "elapsed_ms": float64(1)}}
			raw, _ := json.Marshal(wire)
			var value map[string]any
			_ = json.Unmarshal(raw, &value)
			if err := plan8ScopedLogMatches(value, owner, marker); (err == nil) != test.want {
				t.Fatalf("unexpected body/scope verdict: %v", err)
			}
		})
	}
}
