package datapolicy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectionKeepsOrdinaryLogsTraceDetailsAndMasksCredentials(t *testing.T) {
	data := map[string]any{"body": "request finished password=hidden token=other duration=12", "attributes": map[string]string{"db.statement": "select 1", "http.request.header.authorization": "Bearer abc", "business": "ok"}, "events": []any{map[string]any{"name": "event", "attributes": `{"client_secret":"value","status":"ok"}`}}, "links": []any{map[string]any{"traceId": "linked", "attributes": map[string]any{"key": "api_key", "value": "key-value"}}}, "large": json.Number("18446744073709551615")}
	output, changed, err := Project(data)
	if err != nil || !changed {
		t.Fatal("projection did not mask credentials")
	}
	raw, _ := json.Marshal(output)
	text := string(raw)
	for _, secret := range []string{"hidden", "other", "Bearer abc", "key-value", "client_secret\\\":\\\"value"} {
		if strings.Contains(text, secret) {
			t.Fatalf("credential leaked: %s", secret)
		}
	}
	for _, ordinary := range []string{"request finished", "duration=12", "select 1", "business", "linked", "18446744073709551615"} {
		if !strings.Contains(text, ordinary) {
			t.Fatalf("ordinary data erased: %s", ordinary)
		}
	}
	original, _ := json.Marshal(data)
	if !strings.Contains(string(original), "hidden") {
		t.Fatal("mutated backend result")
	}
}
func TestProjectionDoesNotTreatEveryBodyOrAttributeAsRestricted(t *testing.T) {
	_, changed, err := Project([]map[string]any{{"body": "query finished", "attributes": map[string]any{"service": "api"}, "events": []any{}, "links": []any{}}})
	if err != nil || changed {
		t.Fatal("ordinary telemetry was redacted")
	}
}
