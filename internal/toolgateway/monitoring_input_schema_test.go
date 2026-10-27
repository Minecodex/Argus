package toolgateway

import "testing"

func TestMonitoringPreviewWorkloadSchemaSeparatesObservedIdentity(t *testing.T) {
	doc, err := nativePreviewDocument()
	if err != nil {
		t.Fatal(err)
	}
	schema, err := previewSchema(doc, "MonitoringMutation", "")
	if err != nil {
		t.Fatal(err)
	}
	properties := func(v map[string]any, key string) map[string]any {
		t.Helper()
		p, ok := v["properties"].(map[string]any)[key].(map[string]any)
		if !ok {
			t.Fatalf("missing %s input schema", key)
		}
		return p
	}
	workload := properties(properties(properties(schema, "collection"), "config"), "workload")
	fields := workload["properties"].(map[string]any)
	for _, key := range []string{"namespace", "namespace_uid", "ancestors"} {
		if fields[key] != nil {
			t.Fatalf("Agent input exposes read-only observation %s", key)
		}
	}
	for _, key := range []string{"namespace_id", "type", "name", "uid", "containers"} {
		if fields[key] == nil {
			t.Fatalf("Agent input lost selected identity %s", key)
		}
	}
}
