package argusdev

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceQueryReferencesResolveLiteralAncestorDirectories(t *testing.T) {
	file := "/workspace/queries/metrics/query.json"
	for _, command := range []string{
		"argus-evidence-report queries/metrics/query.json --unit 1",
		"cd /workspace/queries && argus-evidence-report metrics/query.json",
		"mkdir -p /workspace/results && cd /workspace/queries\nargus-evidence-report metrics/query.json --expected-sha256 expected",
		"cd '/workspace/queries'\npython3 -c \"print(open('metrics/query.json').read())\"",
		"cd queries && cd metrics && argus-metric-evidence query.json",
	} {
		if !plan11CommandReferencesFile(command, file) {
			t.Fatal("valid explicit Workspace reference rejected", command)
		}
		if command[:2] != "cd" && command[:5] != "mkdir" {
			continue
		}
		body, _ := json.Marshal([]map[string]any{{"tool": "bash", "status": "succeeded", "exit_code": 0, "partial": false, "input": map[string]any{"command": command}}})
		if !plan11ReadActualQuery(string(body), file) {
			t.Fatal("complete original read receipt rejected", command)
		}
	}
	for _, command := range []string{
		"cd /tmp && argus-evidence-report metrics/query.json",
		"cd /workspace/queries\ncd /tmp\nargus-evidence-report metrics/query.json",
		"cd /workspace/queries\ncd \"$DYNAMIC\"\nargus-evidence-report metrics/query.json",
		"cd /workspace/queries && argus-evidence-report metrics/other.json",
		"cd /workspace/queries && argus-evidence-report metrics/query.json.backup",
		"cd /workspace/queries && argus-evidence-report wrongmetrics/query.json",
	} {
		if plan11CommandReferencesFile(command, file) {
			t.Fatal("wrong/unknown relative source accepted", command)
		}
	}
	for _, outcome := range []map[string]any{{"exit_code": 2, "partial": false}, {"exit_code": 0, "partial": true}} {
		outcome["tool"], outcome["status"] = "bash", "succeeded"
		outcome["input"] = map[string]any{"command": "cd /workspace/queries\nargus-evidence-report metrics/query.json"}
		body, _ := json.Marshal([]map[string]any{outcome})
		if plan11ReadActualQuery(string(body), file) || plan11ComputedStatistics(string(body), file) {
			t.Fatal("failed/partial process accepted through new cwd spelling")
		}
	}
}
