package alerting

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/api/schemas"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestAlertResourceSchemaAndDomainAcceptTheSamePlan8Types(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(schemas.AlertContracts, &document); err != nil {
		t.Fatal(err)
	}
	selector, err := toolruntime.CompileInputSchema(map[string]any{"type": "object", "$ref": "#/$defs/AlertResourceSelector", "$defs": document["$defs"]})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"host", "kubernetes_cluster", "application", "monitoring_target"} {
		scope := map[string]any{"mode": "explicit", "types": []any{kind}, "ids": []any{uuid.NewString()}, "source_ids": []any{}, "matchers": []any{}}
		if err := selector.Validate(scope); err != nil {
			t.Fatalf("API schema rejects domain resource %s: %v", kind, err)
		}
		spec := map[string]any{"enabled": true, "resources": scope, "rules": []map[string]any{{"name": "Actual connections", "expression": "mysql_threads", "query_mode": "numeric", "operator": ">", "threshold": 100000, "recovery_threshold": 99999, "severity": "warning", "unit": ""}}, "mode": "independent", "match_by": []string{"argus_resource_id", "argus_source_id"}, "interval_seconds": 10, "for_seconds": 0, "recovery_seconds": 0, "no_data_behavior": "report", "error_behavior": "report", "channel_ids": []string{}, "route_ids": []string{}, "labels": map[string]string{"suite": "plan8-contract"}}
		raw, _ := json.Marshal(spec)
		if _, err := normalizeMutation(Mutation{Kind: "policy", Operation: "create", Name: "Health contract", Spec: raw}); err != nil {
			t.Fatalf("domain rejects API resource %s: %v", kind, err)
		}
	}
	if err := selector.Validate(map[string]any{"mode": "explicit", "types": []any{"invented_resource"}, "ids": []any{uuid.NewString()}, "matchers": []any{}}); err == nil {
		t.Fatal("unknown resource type was admitted")
	}
}
