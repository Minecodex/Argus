package argusdev

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakj-go/Argus/internal/dashboardcontext"
)

func TestBusinessWorkflowPhaseDeclaresOnlyFiveReadOnlyScenarioTasks(t *testing.T) {
	if len(plan8SelectedModelNames("plan11-workflows")) != 5 {
		t.Fatal("workflowbudget changed")
	}
	for _, name := range []string{"workspace", "owned-linux-hosts", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-business-workflows"} {
		if !plan8StageSelected("plan11-workflows", name) {
			t.Fatal("missingrealprerequisite", name)
		}
	}
	for _, name := range []string{"sdk-application-protocols", "plan11-chat-interactions", "plan11-profile-comparison", "additional-supported-integrations"} {
		if plan8StageSelected("plan11-workflows", name) {
			t.Fatal("undeclaredworkflowselected", name)
		}
	}
	for _, phase := range []string{"all", "plan11-compare", "plan11-metrics", "plan11-interactions"} {
		if plan8StageSelected(phase, "plan11-business-workflows") {
			t.Fatal("oldphasegainednewpaidworkflow", phase)
		}
	}
}

func TestDashboardFilterEquivalentTypedHostScopeRejectsConflicts(t *testing.T) {
	for _, types := range [][]string{nil, {"host"}} {
		if !plan11HostTypeFilterEquivalent(types) {
			t.Fatal("an explicit Host ref can omit a redundant type filter", types)
		}
	}
	for _, types := range [][]string{{"application"}, {"host", "application"}, {"host", "host"}} {
		if plan11HostTypeFilterEquivalent(types) {
			t.Fatal("conflicting or broader type filter accepted", types)
		}
	}
	for _, scope := range []string{dashboardcontext.GeneralDiagnosis, dashboardcontext.DashboardAnalysis} {
		suffix := plan11ScopeBrowserSuffix(scope)
		if suffix != kubernetesNameForDev(suffix) {
			t.Fatal("actual browser suffix rejected by shared driver", scope, suffix)
		}
	}
}

// Explicit replay of immutable structured exports, with no provider calls or
// modification of the original R66 machine verdict.
func TestDashboardFilterRecordedBusinessEvidenceReplay(t *testing.T) {
	root := os.Getenv("ARGUS_PLAN11_FILTER_REPLAY_DIR")
	if root == "" {
		t.Skip("requires explicit recorded native tool exports")
	}
	var workflow struct {
		Empty string `json:"metadata_only_host_without_sources"`
		Cases []struct {
			Name  string
			Calls []plan8ModelToolFact
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "plan11-business-workflows.json"))
	if err != nil || json.Unmarshal(raw, &workflow) != nil || workflow.Empty == "" || len(workflow.Cases) != 2 {
		t.Fatal("invalid recorded workflow", err)
	}
	for _, row := range workflow.Cases {
		t.Run(row.Name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, row.Name+"-tools.json"))
			if err != nil {
				t.Fatal(err)
			}
			var calls []struct {
				Tool  string
				Input struct {
					Category, Name string
					Arguments      struct{ Resources []struct{ Type, ID string } }
				}
			}
			if json.Unmarshal(data, &calls) != nil {
				t.Fatal("invalid recorded calls")
			}
			host := ""
			for _, c := range calls {
				if c.Tool == "tool.invoke" && c.Input.Category == "dashboard" && c.Input.Name == "list" {
					for _, ref := range c.Input.Arguments.Resources {
						if ref.Type == "host" && ref.ID != workflow.Empty {
							host = ref.ID
						}
					}
				}
			}
			if host == "" {
				t.Fatal("missing exact recorded Host")
			}
			if err = plan11DashboardFilterTraceAllowed(string(data)); err != nil {
				t.Fatal(err)
			}
			if err = plan11VerifyDashboardFilterCalls(string(data), host, map[string]any{}, workflow.Empty); err != nil {
				t.Fatal(err)
			}
			// An unauthorized ID or conflicting resource type cannot become an
			// equivalent scope simply by omitting resource_types.
			if plan11VerifyDashboardFilterCalls(string(data), "wrong-host", map[string]any{}, workflow.Empty) == nil {
				t.Fatal("wrong Host accepted")
			}
		})
	}
}

func TestDashboardFilterBusinessEvidenceRequiresExactHostFiltersAndActualEmpty(t *testing.T) {
	makeCall := func(signal string, items []any, complete bool) map[string]any {
		return map[string]any{"tool": "tool.invoke", "status": "succeeded", "input": map[string]any{"category": "dashboard", "name": "list", "arguments": map[string]any{"resource_types": []string{"host"}, "resources": []any{map[string]any{"type": "host", "id": "owned"}}, "signals": []string{signal}, "integrations": []string{"host_metrics"}}}, "projection": map[string]any{"summary": map[string]any{"complete": complete, "items": items}}}
	}
	metric := makeCall("metrics", []any{map[string]any{"suitability": "applicable", "sampling_performed": false, "data_state": "not_checked"}}, true)
	logs := makeCall("logs", []any{}, true)
	raw, _ := json.Marshal([]any{metric, logs})
	if err := plan11VerifyDashboardFilterCalls(string(raw), "owned", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	partial := makeCall("metrics", []any{map[string]any{"suitability": "partial", "sampling_performed": false, "data_state": "not_checked"}}, true)
	partial["input"].(map[string]any)["arguments"].(map[string]any)["resources"] = []any{map[string]any{"type": "host", "id": "owned"}, map[string]any{"type": "host", "id": "empty"}}
	raw, _ = json.Marshal([]any{metric, logs, partial})
	if err := plan11VerifyDashboardFilterCalls(string(raw), "owned", map[string]any{}, "empty"); err != nil {
		t.Fatal("valid partial applicability not certified", err)
	}
	logs["projection"].(map[string]any)["summary"].(map[string]any)["complete"] = false
	raw, _ = json.Marshal([]any{metric, logs})
	if plan11VerifyDashboardFilterCalls(string(raw), "owned", map[string]any{}) == nil {
		t.Fatal("partialemptytreatedascomplete no-match")
	}
	if !errors.Is(plan11DashboardFilterTraceAllowed(`[{"tool":"connect","status":"failed"}]`), errPlan11NavigationBoundary) {
		t.Fatal("failedremoteattemptdidnottripreadonlyboundary")
	}
}
