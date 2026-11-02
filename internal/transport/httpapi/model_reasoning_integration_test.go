package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type reasoningSettingsTransport func(*http.Request) (*http.Response, error)

type reasoningSettingsResolver struct{}

func (reasoningSettingsResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

func (f reasoningSettingsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Actual HTTP contracts, PostgreSQL revisions and encrypted credentials with a
// controlled provider. No live supplier support or paid calls are claimed.
func TestModelThinkingDepthHTTPRevisionLifecycle(t *testing.T) {
	f := newAuditWorkflowFixture(t)
	key := bytes.Repeat([]byte{23}, 32)
	keyPath := filepath.Join(t.TempDir(), "reasoning-keyring.json")
	keyData, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(key)}})
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	observed := []string{}
	client := &http.Client{Transport: reasoningSettingsTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		effort, _ := body["reasoning_effort"].(string)
		observed = append(observed, effort)
		if effort == "high" {
			return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("unsupported fixture depth"))}, nil
		}
		parameter := modelprovider.BudgetMaxTokens
		if _, ok := body[parameter]; !ok {
			parameter = modelprovider.BudgetMaxCompletionTokens
		}
		messages, _ := json.Marshal(body["messages"])
		finish, text, output := "stop", "OK", 8
		delta := map[string]any{}
		if strings.Contains(string(messages), modelprovider.OutputBudgetProbeMarker) {
			finish, output = "length", int(body[parameter].(float64))
		} else if _, ok := body["tools"]; ok {
			finish = "tool_calls"
			delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "reasoning-compat", "type": "function", "function": map[string]any{"name": "compatibility_probe", "arguments": "{}"}}}
		} else if _, ok := body["response_format"]; ok {
			text = `{"ok":true}`
		}
		if finish == "stop" {
			delta["content"] = text
		}
		wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 32, "completion_tokens": output}})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(wire) + "\n\ndata: [DONE]\n\n"))}, nil
	})}
	origins := []string{f.server.URL}
	auth := SetupHandler{Config: config.Server{AllowedOrigins: origins}, Identity: identity.Service{Store: f.store}}
	enterprise := EnterpriseIdentityHandler{Auth: auth, Service: identity.EnterpriseService{Store: f.store}, Cursor: pagination.Signer{Key: key}}
	handler := ModelHandler{Identity: enterprise, Service: model.Service{Store: f.store, Idempotency: f.domain.Actions.Idempotency, Keyring: keyring,
		Tester: model.ProviderTester{Policy: modelprovider.PublicEndpointPolicy{Resolver: reasoningSettingsResolver{}}, Client: client}}}
	f.server.Config.Handler = NewRouterWithOptions(RouterOptions{Setup: &auth, EnterpriseIdentity: &enterprise, Model: &handler, AllowedOrigins: origins})
	decode := func(raw []byte) map[string]any {
		t.Helper()
		if bytes.Contains(raw, []byte("fixture-write-only-key")) || bytes.Contains(raw, []byte("api_key")) {
			t.Fatal("credential leaked in model response")
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	assertDepth := func(raw []byte, want string, revision float64) {
		t.Helper()
		value := decode(raw)
		capabilities := value["capabilities"].(map[string]any)
		actual, _ := capabilities["reasoning_effort"].(string)
		if actual != want || value["revision"] != revision {
			t.Fatalf("wrong model depth or revision: %s", raw)
		}
	}
	input := map[string]any{"name": "Thinking depth fixture", "base_url": "https://api.apimart.ai/v1", "model_id": "controlled-model", "api_protocol": "chat_completions", "api_key": "fixture-write-only-key", "context_window_tokens": 131072, "max_output_tokens": 8192, "input_price_per_million": 1, "output_price_per_million": 2, "reasoning_effort": "medium"}
	created := decode(f.call("POST", "/enterprise/ai-models/test-and-create", input, 201, uuid.NewString()))["model"].(map[string]any)
	id := uuid.MustParse(created["id"].(string))
	modelPath := "/enterprise/ai-models/" + id.String()
	assertDepth(f.call("GET", modelPath, nil, 200, ""), "medium", 1)
	conversationID, runID := uuid.New(), uuid.New()
	f.exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Frozen model depth',$4)", conversationID, f.enterprise, f.user, id)
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", runID, conversationID, f.enterprise, f.user, id)
	// Omission preserves the setting; explicit provider_default clears it.
	assertDepth(f.call("PUT", modelPath, map[string]any{"name": "Renamed", "expected_version": 1}, 200, uuid.NewString()), "medium", 2)
	decode(f.call("POST", modelPath+"/test", nil, 200, uuid.NewString()))
	for _, effort := range observed {
		if effort != "medium" {
			t.Fatal("create, update or re-test lost configured depth")
		}
	}
	f.call("PUT", modelPath, map[string]any{"reasoning_effort": "high", "expected_version": 2}, 422, uuid.NewString())
	assertDepth(f.call("GET", modelPath, nil, 200, ""), "medium", 2)
	input["reasoning_effort"] = "high"
	f.call("POST", "/enterprise/ai-models/test-and-create", input, 422, uuid.NewString())
	var modelsCount, revisionsCount int
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM ai_models WHERE enterprise_id=$1", f.enterprise).Scan(&modelsCount); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM ai_model_revisions WHERE model_id=$1", id).Scan(&revisionsCount); err != nil {
		t.Fatal(err)
	}
	if modelsCount != 1 || revisionsCount != 2 {
		t.Fatal("failed compatibility created a model or revision")
	}
	assertDepth(f.call("PUT", modelPath, map[string]any{"reasoning_effort": "provider_default", "expected_version": 2}, 200, uuid.NewString()), "", 3)
	if observed[len(observed)-1] != "" {
		t.Fatal("default depth still sent an explicit provider parameter")
	}
	var frozenRevision int32
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT model_revision FROM runs WHERE id=$1", runID).Scan(&frozenRevision); err != nil {
		t.Fatal(err)
	}
	old, err := f.store.Queries.GetEnabledAIModelRevision(t.Context(), db.GetEnabledAIModelRevisionParams{ModelID: id, EnterpriseID: f.enterprise, Revision: frozenRevision})
	if err != nil {
		t.Fatal(err)
	}
	effort, err := modelprovider.ReasoningEffortForCapabilities(modelprovider.Protocol(old.ApiProtocol), old.Capabilities)
	if err != nil || effort != "medium" || frozenRevision != 1 {
		t.Fatal("active run depth changed with later model settings")
	}
}
