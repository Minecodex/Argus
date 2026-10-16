package skywalking

import "testing"

func TestPresentationResultsRespectRootsAliasesFragmentsAndSelection(t *testing.T) {
	doc := `query Selected { services: queryAPMServices { rows { ...Fields } } named: queryBasicTracesByName(serviceName:"orders") { traces { id:traceId rootService } } hidden: queryAPMServices @skip(if:true) { rows { serviceName } } } fragment Fields on APMRow { name:serviceName samples:sampleCount }`
	if err := ValidateInputs(doc, "Selected", nil); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"services": map[string]any{"rows": []any{map[string]any{"name": "orders", "samples": float64(427)}}}, "named": map[string]any{"traces": []any{map[string]any{"id": "trace-1", "rootService": "orders"}}}}
	views, err := PresentationResults(doc, "Selected", payload)
	if err != nil || len(views) != 2 {
		t.Fatal(views, err)
	}
	first := views[0].(map[string]any)
	row := first["rows"].([]any)[0].(map[string]any)
	if first["key"] != "services" || first["kind"] != "apm_services" || row["serviceName"] != "orders" || row["sampleCount"] != float64(427) || row["name"] != nil {
		t.Fatal(views)
	}
	if views[1].(map[string]any)["rows"].([]any)[0].(map[string]any)["traceId"] != "trace-1" {
		t.Fatal(views)
	}
	if payload["services"].(map[string]any)["rows"].([]any)[0].(map[string]any)["name"] != "orders" {
		t.Fatal("provider result mutated")
	}
}

func TestPresentationMergesRepeatedSelectionsWithoutDuplicatingRoots(t *testing.T) {
	doc := `query { queryAPMServices { rows { serviceName } } queryAPMServices { rows { sampleCount } } }`
	if err := ValidateInputs(doc, "", nil); err != nil {
		t.Fatal(err)
	}
	views, err := PresentationResults(doc, "", map[string]any{"queryAPMServices": map[string]any{"rows": []any{map[string]any{"serviceName": "orders", "sampleCount": float64(427)}}}})
	if err != nil || len(views) != 1 {
		t.Fatal(views, err)
	}
	row := views[0].(map[string]any)["rows"].([]any)[0].(map[string]any)
	if row["serviceName"] != "orders" || row["sampleCount"] != float64(427) {
		t.Fatal(row)
	}
}
