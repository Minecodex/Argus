//go:build m4e2e

package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	modelservice "github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type usageFixtureResolver struct{}

func (usageFixtureResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

func TestCompactionPersistsUsageProvenance(t *testing.T) {
	for _, mode := range []string{"missing", "partial", "zero", "complete", "failed", "contradictory"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecoveryFixture(t)
			received := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				received <- request
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"A compact summary of prior conversation.\"},\"finish_reason\":\"stop\"}]}\n\n")
				if mode == "contradictory" {
					fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"prompt_tokens_details\":{\"cached_tokens\":80}}}\n\n")
				}
				values := map[string]any{}
				if mode == "partial" || mode == "complete" || mode == "failed" {
					values["prompt_tokens"] = 123
				}
				if mode == "complete" || mode == "failed" {
					values["completion_tokens"] = 17
					values["prompt_tokens_details"] = map[string]any{"cached_tokens": 51}
				}
				if mode == "contradictory" {
					values["prompt_tokens"] = 50
					values["completion_tokens"] = 8
				}
				if mode == "zero" {
					values["prompt_tokens"] = 0
					values["completion_tokens"] = 0
					values["prompt_tokens_details"] = map[string]any{"cached_tokens": 0}
				}
				data, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": values})
				fmt.Fprintf(w, "data: %s\n\n", data)
				if mode == "failed" {
					fmt.Fprint(w, "data: {\"error\":{\"message\":\"fixture failure\"}}\n\n")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			endpoint := strings.Replace(server.URL, "127.0.0.1", "argus-replay-model", 1)
			revision := uuid.New()
			f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT $2,id,enterprise_id,revision,$3,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m, revision, endpoint)
			if mode == "contradictory" {
				f.exec("UPDATE ai_model_revisions SET input_price_per_million=10,output_price_per_million=20 WHERE id=$1", revision)
			}
			keyPath := filepath.Join(t.TempDir(), "test-keyring.json")
			keyData, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}})
			if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
				t.Fatal(err)
			}
			keyring, err := secret.LoadKeyring(keyPath)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := keyring.Encrypt([]byte("audit-fixture-only"), []byte("argus.model_credential/v1\x00"+f.e.String()+"\x00"+revision.String()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.store.Queries.CreateAIModelCredential(t.Context(), db.CreateAIModelCredentialParams{ID: uuid.New(), ModelRevisionID: revision, EnterpriseID: f.e, Provider: envelope.Provider, KeyID: envelope.KeyID, KeyVersion: int32(envelope.KeyVersion), WrappedDek: envelope.WrappedDEK, WrapNonce: envelope.WrapNonce, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, ValueHash: envelope.ValueHash})
			if err != nil {
				t.Fatal(err)
			}
			scope, err := presentation.Scope(t.Context(), f.store, f.e, f.u)
			if err != nil {
				t.Fatal(err)
			}
			compactor := Compactor{Store: f.store, Models: modelservice.Service{Store: f.store, Keyring: keyring}, EndpointPolicy: modelprovider.PublicEndpointPolicy{Resolver: usageFixtureResolver{}}}
			summary, tokens, err := compactor.generateSummary(t.Context(), f.run(), []db.ConversationEvent{{Sequence: 1, EventType: "user_message", Payload: []byte(`{"content":"Please retain the business context for the next turn."}`)}}, scope, conversation.WorkspaceContext{Status: "not_created"}, ContextSource{FromSequence: 1})
			if (err != nil) != (mode == "failed" || mode == "contradictory") {
				t.Fatalf("mode=%s error=%v", mode, err)
			}
			request := <-received
			options, _ := request["stream_options"].(map[string]any)
			if options["include_usage"] != true {
				t.Fatal("compactor omitted usage request")
			}
			var input, output int64
			var inputSource, outputSource, settlement, status string
			err = f.store.Pool.QueryRow(t.Context(), "SELECT c.input_tokens,c.output_tokens,c.input_usage_source,c.output_usage_source,r.usage_source,c.status FROM model_calls c JOIN model_quota_reservations r ON r.model_call_id=c.id WHERE c.run_id=$1", f.r).Scan(&input, &output, &inputSource, &outputSource, &settlement, &status)
			if err != nil {
				t.Fatal(err)
			}
			expectedInput, expectedOutput := "provider", "provider"
			if mode == "missing" {
				expectedInput = "estimated"
			}
			if mode == "partial" || mode == "missing" {
				expectedOutput = "estimated"
			}
			if mode == "contradictory" {
				expectedInput, expectedOutput = "invalid", "invalid"
			}
			if inputSource != expectedInput || outputSource != expectedOutput {
				t.Fatalf("sources %s/%s", inputSource, outputSource)
			}
			var cached int64
			var cachedSource string
			if err := f.store.Pool.QueryRow(t.Context(), "SELECT cached_input_tokens,cached_input_usage_source FROM model_calls WHERE run_id=$1", f.r).Scan(&cached, &cachedSource); err != nil {
				t.Fatal(err)
			}
			if (mode == "complete" || mode == "failed") && (cached != 51 || cachedSource != "provider") {
				t.Fatal("cached input usage not persisted")
			}
			if mode == "zero" && (cached != 0 || cachedSource != "provider") {
				t.Fatal("reported zero cache usage lost")
			}
			if (mode == "missing" || mode == "partial") && (cached != 0 || cachedSource != "missing") {
				t.Fatal("missing cache usage converted into reported zero")
			}
			if mode == "zero" && (input != 0 || output != 0 || tokens != 0) {
				t.Fatal("reported zero overwritten")
			}
			if (mode == "missing" || mode == "partial") && (settlement != "estimated" || tokens != 0) {
				t.Fatal("estimate presented as actual summary usage or settlement")
			}
			if mode == "complete" && (input != 123 || output != 17 || tokens != 17 || settlement != "provider") {
				t.Fatal("reported usage lost")
			}
			if mode == "failed" && status != "failed" {
				t.Fatal("failed call marked succeeded")
			}
			if mode == "contradictory" {
				if summary != "" || tokens != 0 || input != 50 || output != 8 || cached != 80 || cachedSource != "invalid" || settlement != "invalid" || status != "failed" {
					t.Fatal("invalid compaction usage was accepted")
				}
				var safe bool
				if err := f.store.Pool.QueryRow(t.Context(), "SELECT c.error_code='MODEL_USAGE_INVALID' AND q.settled_amount=q.reserved_amount AND q.reserved_amount>0 AND c.amount=q.reserved_amount AND s.status='failed' FROM model_calls c JOIN model_quota_reservations q ON q.model_call_id=c.id JOIN run_steps s ON s.id=c.step_id WHERE c.run_id=$1", f.r).Scan(&safe); err != nil || !safe {
					t.Fatalf("invalid settlement/step: %v", err)
				}
			}
			if mode == "contradictory" {
				f.exec("UPDATE runs SET status='waiting_system',stop_reason='context_compaction',tool_snapshot='{\"version\":\"argus.model_tool_set/v1\",\"tools\":[]}' WHERE id=$1", f.r)
				for _, event := range []struct{ kind, content string }{{"user_message", strings.Repeat("x", 25000)}, {"assistant_message", strings.Repeat("a", 10000)}, {"model_usage", ""}, {"user_message", "continue"}} {
					if _, err := conversation.AppendEvent(t.Context(), f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: event.kind, ActorType: "model", Payload: map[string]any{"content": event.content, "authorization_scope": scope}, Classification: "internal"}); err != nil {
						t.Fatal(err)
					}
				}
				if err := compactor.Handle(t.Context(), f.task("compaction", "hard_limit")); err != nil {
					t.Fatal(err)
				}
				current := f.run()
				if current.Status != "failed" || current.ErrorCode.String != "MODEL_USAGE_INVALID" {
					t.Fatalf("invalid hard compaction did not stop Run: %s/%s", current.Status, current.ErrorCode.String)
				}
				if f.count("SELECT count(*) FROM context_snapshots WHERE conversation_id=$1", f.c) != 0 || f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='failed'", f.r) != 1 {
					t.Fatal("invalid hard compaction snapshot/terminal event")
				}
			}
			if mode != "failed" && mode != "contradictory" && summary == "" {
				t.Fatal("summary missing")
			}
		})
	}
}
