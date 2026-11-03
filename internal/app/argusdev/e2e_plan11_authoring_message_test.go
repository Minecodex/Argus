package argusdev

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Capture the real outgoing message, then stop before any model/DB dependency.
// This detects an omitted creation context before a costly full-stack run.
func TestBusinessWorkflowAuthoringReplaySendsExplicitCreateMode(t *testing.T) {
	draft, conversation, run, model, host := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	capturedMode := ""
	messages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/dashboard-drafts":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": draft, "draft_version": 1})
		case r.Method == http.MethodPost && r.URL.Path == "/conversations":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": conversation})
		case r.Method == http.MethodPost && r.URL.Path == "/conversations/"+conversation+"/messages":
			var body struct {
				DashboardContext struct {
					Mode string `json:"mode"`
				} `json:"dashboard_context"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				w.WriteHeader(400)
				return
			}
			capturedMode = body.DashboardContext.Mode
			messages++
			w.WriteHeader(400) // Controlled stop after capturing the outgoing body.
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "CONTROLLED_PREFLIGHT_STOP"})
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+run:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": run, "status": "failed"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	env := &E2EEnvironment{Endpoints: &E2EEndpoints{EnterpriseOrigin: server.URL}, State: NewScenarioState("create-mode-preflight")}
	env.State.HTTP = NewScenarioHTTP(server.URL, t.TempDir(), nil)
	env.State.Values["m4_model_id"] = model
	fixtures, err := plan11RecordedAuthoringSpecs()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&App{}).plan11AuthoringReplayCase(t.Context(), env, &plan6HostLoop{HostID: host}, fixtures[0])
	if err == nil {
		t.Fatal("controlled test must stop before model/DB dependencies")
	}
	if messages != 1 || capturedMode != "create" {
		t.Fatalf("actual outgoing message missing authoring mode: messages=%d mode=%q", messages, capturedMode)
	}
}
