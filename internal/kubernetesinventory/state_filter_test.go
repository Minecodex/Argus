package kubernetesinventory

import (
	"encoding/json"
	"testing"
)

func TestStateFiltersShareSummarySemantics(t *testing.T) {
	definition, _ := Lookup("pod")
	for _, item := range []struct{ raw, state string }{
		{`{"status":{"phase":"Running","conditions":[{"type":"Ready","status":"False"}]}}`, "warning"},
		{`{"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}`, "healthy"},
		{`{"status":{"phase":"Succeeded"}}`, "completed"},
		{`{}`, "unknown"},
	} {
		object := Object{Definition: definition, Data: json.RawMessage(item.raw)}
		if !MatchesState(object, item.state) || !MatchesState(object, "") {
			t.Fatal(item.state)
		}
		if MatchesState(object, "attention") != (item.state == "warning" || item.state == "unknown") {
			t.Fatal("attention semantics drifted", item.state)
		}
	}
	if ValidStateFilter("healthy OR 1=1") {
		t.Fatal("unknown state accepted")
	}
}
func TestPodRequestsUseKubernetesSchedulingSemantics(t *testing.T) {
	definition, _ := Lookup("pod")
	object := Object{Definition: definition, Data: json.RawMessage(`{"spec":{"containers":[{"name":"api","resources":{"requests":{"cpu":"100m","memory":"32Mi"}}}],"initContainers":[{"name":"init","resources":{"requests":{"cpu":"200m","memory":"16Mi"}}}],"overhead":{"cpu":"10m","memory":"1Mi"}}}`)}
	summary := Summarize(object)
	if summary.Fields["cpu_request_millicores"] != "210" || summary.Fields["memory_request_bytes"] != "34603008" {
		t.Fatal(summary)
	}
}

func TestResourceDefinitionsHaveUniqueStableKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Definitions() {
		if seen[d.Key] {
			t.Fatal("duplicate resource type", d.Key)
		}
		seen[d.Key] = true
	}
}
