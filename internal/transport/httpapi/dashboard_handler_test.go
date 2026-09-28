package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestDashboardBindingAndAuthorizationRequestContracts(t *testing.T) {
	id := uuid.NewString()
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/v1/enterprise/data-authorizations/user/" + id + "?resource_type=dashboard", "", 204},
		{http.MethodGet, "/api/v1/enterprise/data-authorizations/user/" + id + "?resource_type=metrics", "", 400},
		{http.MethodPost, "/api/v1/enterprise/data-authorizations/user/" + id, `{"resource_type":"dashboard","resource_ids":["` + id + `"],"remove":false,"expected_version":1}`, 204},
		{http.MethodPost, "/api/v1/hosts/" + id + "/dashboard-bindings/preview", `{"operation":"attach","dashboard_id":"` + id + `","expected_dashboard_version":1,"expected_resource_version":1}`, 204},
		{http.MethodPost, "/api/v1/kubernetes/clusters/" + id + "/dashboard-bindings/preview", `{"operation":"detach","dashboard_id":"` + id + `","expected_dashboard_version":1,"expected_resource_version":1,"binding_id":"` + id + `","expected_binding_version":1}`, 204},
		{http.MethodPost, "/api/v1/hosts/" + id + "/dashboard-bindings/preview", `{"operation":"attach","dashboard_id":"` + id + `","expected_dashboard_version":1}`, 400},
		{http.MethodPost, "/api/v1/dashboards/catalog/query", `{"signal":"logs","kind":"fields","source_binding":{"source_type":"otlp","capability_version":"v1"},"from":"2026-09-25T00:00:00Z","to":"2026-09-25T01:00:00Z","selected_values":[],"filters":[],"resource_ids":[],"limit":25}`, 204},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries", `{"dashboard_id":"` + id + `","parameters":{"resource_ids":[],"panel_ids":[],"variables":{},"local_values":{}}}`, 204},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries", `{"dashboard_id":"` + id + `","query":"arbitrary raw query","parameters":{"resource_ids":[],"panel_ids":[],"variables":{},"local_values":{}}}`, 400},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries/" + id + "/resume", `{"expected_version":2}`, 204},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries/" + id + "/resume", `{}`, 400},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries", `{"dashboard_id":"` + id + `","parameters":{},"drilldown":{"parent_job_id":"` + id + `","panel_id":"traces","drilldown_id":"detail","values":{},"expand_authorized_resources":false}}`, 204},
		{http.MethodPost, "/api/v1/conversations/" + id + "/dashboard-queries", `{"dashboard_id":"` + id + `","parameters":{},"drilldown":{"parent_job_id":"` + id + `","panel_id":"traces","drilldown_id":"detail","values":{},"expand_authorized_resources":false,"query":"override"}}`, 400},
	} {
		t.Run(test.method+test.path+test.body, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
			request.Header.Set("Idempotency-Key", "dashboard-contract-test-key")
			response := httptest.NewRecorder()
			openAPIRequestValidationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDashboardPublicationUsesPublicActionDiffContract(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	changes, _ := json.Marshal([]dashboard.Change{{Kind: "change", Text: "query: old → new"}})
	value := convertPending[api.PendingActionPublicSchema](db.PendingAction{ID: uuid.New(), ActionRef: "act_dashboard", ActionType: "telemetry.dashboard.publish", Title: "Payment", Summary: "Publish", Risk: "write", Status: "awaiting_confirmation", Preview: []byte(`{"before_json":"null","after_json":"{}"}`), Diff: changes, CreatedAt: now, UpdatedAt: now, ExpiresAt: now})
	if len(value.Diff) != 1 || value.Diff[0].Text != "query: old → new" || len(value.AvailableActions) != 2 || value.ActionRef != "act_dashboard" {
		t.Fatalf("pending action conversion lost review or confirmation fields: %+v", value)
	}
}

func TestConvertedPanelDTOHasNonNullEditorCollections(t *testing.T) {
	input := dashboard.Panel{ID: "metric", AuthoringMode: "dsl", Signal: "metrics", Targets: []dashboard.Target{{ID: "main", Language: "promql", QueryMode: "instant", SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: "sum(cpu)"}}}}}
	result := dashboard.ConvertPanel(dashboard.ConvertPanelInput{Panel: input, Mode: "builder"})
	if !result.Converted {
		t.Fatal(result.Issues)
	}
	view := dashboardConvert[api.DashboardConvertedPanel](result)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	panel := payload["panel"].(map[string]any)
	for _, key := range []string{"detail_query_targets", "drilldowns", "local_filters", "thresholds", "applicable_resource_types"} {
		if _, ok := panel[key].([]any); !ok {
			t.Fatalf("%s is not an array: %s", key, encoded)
		}
	}
	target := panel["targets"].([]any)[0].(map[string]any)
	if _, ok := target["parameter_bindings"].([]any); !ok {
		t.Fatal("parameter_bindings is not an array")
	}
	builder := target["source_definition"].(map[string]any)["builder"].(map[string]any)
	for _, key := range []string{"filters", "group_by"} {
		if _, ok := builder[key].([]any); !ok {
			t.Fatalf("%s is not an array", key)
		}
	}
}

func TestDashboardRevisionDoesNotExposeEditorsSampleData(t *testing.T) {
	spec, _ := json.Marshal(dashboard.EmptySpec())
	view := dashboardRevisionView(db.DashboardRevision{Spec: spec, SampleReport: []byte(`{"status":"success","data":[{"body":"private observation"}]}`), ValidationReport: []byte(`{"valid":true,"issues":[],"compiler_version":"test"}`)})
	if _, ok := view.Sample["data"]; ok {
		t.Fatal("dashboard configuration read leaked sampled resource data")
	}
	if view.Sample["status"] != "requires_resource_authorization" {
		t.Fatalf("sample availability was misrepresented: %+v", view.Sample)
	}
}

func TestDashboardParameterExecutionContractRoundTrip(t *testing.T) {
	input := dashboard.ExecutionInput{Variables: map[string]dashboard.Selection{"env": {All: true}}, LocalValues: map[string]map[string]dashboard.Selection{"logs": {"term": {Values: []string{"a\"b"}}}}}
	transport := dashboardConvert[api.DashboardExecutionInput](input)
	back := dashboardConvert[dashboard.ExecutionInput](transport)
	if !back.Variables["env"].All || back.LocalValues["logs"]["term"].Values[0] != "a\"b" {
		t.Fatal("execution input DTO lost typed selections")
	}
	output := dashboard.Execution{Variables: input.Variables, LocalValues: input.LocalValues, VariableCandidates: map[string]dashboard.CandidateState{"env": {Status: "success", Complete: true, Reset: true, Values: []string{}, SelectedExists: map[string]bool{"gone": false}}}}
	response := dashboardConvert[api.DashboardExecution](output)
	actual := dashboardConvert[dashboard.Execution](response)
	state := actual.VariableCandidates["env"]
	if !state.Reset || !state.Complete || state.Status != "success" || len(state.SelectedExists) != 1 {
		t.Fatal("execution DTO lost candidate proof")
	}
}
