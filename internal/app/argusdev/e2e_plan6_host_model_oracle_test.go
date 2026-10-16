package argusdev

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestHostAlertConclusionRejectsInventedValuesAndLifecycles(t *testing.T) {
	items := []any{}
	for _, id := range []string{"first", "second"} {
		items = append(items, map[string]any{"id": id, "policy_id": "policy", "resource_ids": []any{"host"}, "lifecycle": "firing", "state": map[string]any{"kind": "condition", "value": 12.5}})
	}
	trace, _ := json.Marshal([]any{map[string]any{"tool": "tool.invoke", "status": "succeeded", "output": map[string]any{"page": map[string]any{"items": items}}}})
	valid := "```json\n" + `{"host_id":"host","instances":[{"id":"first","lifecycle":"firing","value":12.5},{"id":"second","lifecycle":"firing","value":12.5}]}` + "\n```"
	if err := plan6HostVerifyAlertConclusion(valid, string(trace), "host", "policy", "active"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{strings.Replace(valid, "12.5", "99", 1), strings.Replace(valid, "firing", "resolved", 1), strings.Replace(valid, "second", "first", 1), strings.Replace(valid, `"host"`, `"other"`, 1)} {
		if err := plan6HostVerifyAlertConclusion(invalid, string(trace), "host", "policy", "active"); err == nil {
			t.Fatal("unsupported model conclusion passed", invalid)
		}
	}
	// The real model can read fresher instance details after listing events.
	// Those observed values are valid evidence too; invented values remain false.
	detail := map[string]any{"id": "first", "policy_id": "policy", "resource_ids": []any{"host"}, "lifecycle": "firing", "state": map[string]any{"kind": "condition", "value": 24.5}}
	trace, _ = json.Marshal([]any{map[string]any{"tool": "tool.invoke", "status": "succeeded", "output": map[string]any{"page": map[string]any{"items": items}}}, map[string]any{"tool": "tool.invoke", "status": "succeeded", "output": map[string]any{"instance": detail}}})
	if err := plan6HostVerifyAlertConclusion(strings.Replace(valid, "12.5", "24.5", 1), string(trace), "host", "policy", "active"); err != nil {
		t.Fatal("actual fresher event.get value rejected", err)
	}
	health := map[string]any{"id": "health", "policy_id": "policy", "resource_ids": []any{"host"}, "lifecycle": "resolved", "state": map[string]any{"kind": "health", "value": nil}}
	trace, _ = json.Marshal([]any{map[string]any{"tool": "tool.invoke", "status": "succeeded", "output": map[string]any{"page": map[string]any{"items": append(items, health)}}}})
	withHealth := strings.Replace(valid, `}]}`, `},{"id":"health","lifecycle":"resolved","value":null}]}`, 1)
	if err := plan6HostVerifyAlertConclusion(withHealth, string(trace), "host", "policy", "active"); err != nil {
		t.Fatal("actual recovered NoData history incorrectly rejected", err)
	}
	if err := plan6HostVerifyAlertConclusion(valid, string(trace), "host", "policy", "active"); err == nil {
		t.Fatal("omission of observed health history accepted")
	}
}

func TestRealH08HostAlertConclusionPreservesRecoveredHealthAndFreshDetails(t *testing.T) {
	raw, err := os.ReadFile("testdata/host-alert-h08-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		HostID       string `json:"host_id"`
		PolicyID     string `json:"policy_id"`
		Phase, Reply string
		Calls        json.RawMessage
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if err = plan6HostVerifyAlertConclusion(fixture.Reply, string(fixture.Calls), fixture.HostID, fixture.PolicyID, fixture.Phase); err != nil {
		t.Fatal("correct real GLM conclusion rejected", err)
	}
}
