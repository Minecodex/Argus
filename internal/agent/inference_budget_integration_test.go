//go:build m4e2e

package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	modelservice "github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type inferenceBudgetFixtureTools struct{ invoked *atomic.Int32 }

func (p inferenceBudgetFixtureTools) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	part := toolruntime.Contribution{NativeCatalog: "bounded-inference-fixture"}
	for _, name := range []string{"read", "read_other", "read_third"} {
		part.Tools = append(part.Tools, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: name, Description: "Read a controlled observation.", Schema: map[string]any{"type": "object", "additionalProperties": false}}, Source: "argus", ReadOnly: true, Version: "1"}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			p.invoked.Add(1)
			return toolruntime.Result{Data: map[string]any{"observed": true}}, nil
		}})
	}
	return part, nil
}

func TestPostgresInferenceBudgetFinalizationAndDurableRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		used                  int
		ignoresTools, pending bool
		wantCalls, wantTools  int
		status                string
	}{
		{"fresh", 0, false, false, 24, 23, "succeeded"},
		{"resumed_final_turn", 23, false, false, 1, 0, "succeeded"},
		{"provider_ignores_withheld_tools", 0, true, false, 24, 23, "failed"},
		{"exhausted_run", 24, false, false, 0, 0, "failed"},
		{"pending_confirmation_remains_paused", 24, false, true, 0, 0, "waiting_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx := t.Context()
			var calls, invoked atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []modelprovider.Message `json:"messages"`
					Tools    []json.RawMessage       `json:"tools"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid controlled model request")
					w.WriteHeader(400)
					return
				}
				index := int(calls.Add(1))
				used := tc.used + index - 1
				if len(request.Messages) == 0 || !strings.Contains(request.Messages[0].Content, fmt.Sprintf("%d of 24 calls have been used", used)) {
					t.Error("remaining budget was not sourced from durable inference count")
				}
				final := used == 23
				if (len(request.Tools) == 0) != final {
					t.Error("only final response must withhold advertised tools")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				var delta map[string]any
				reason := "stop"
				if !final || tc.ignoresTools {
					reason = "tool_calls"
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("read-%d", index), "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}}}}
				} else {
					delta = map[string]any{"content": "Observed information summarized; any unverified work remains explicitly incomplete."}
				}
				chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\ndata: [DONE]\n\n", chunk)
			}))
			defer server.Close()
			endpoint := strings.Replace(server.URL, "127.0.0.1", "argus-replay-model", 1)
			f.exec(`UPDATE ai_models SET context_window_tokens=262144,capabilities='{"output_token_parameter":"max_tokens"}' WHERE id=$1`, f.m)
			revision := uuid.New()
			f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT $2,id,enterprise_id,revision,$3,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m, revision, endpoint)
			keyPath := filepath.Join(t.TempDir(), "keyring.json")
			encoded, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x43}, 32))}})
			if err := os.WriteFile(keyPath, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			keyring, err := secret.LoadKeyring(keyPath)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := keyring.Encrypt([]byte("controlled-budget-model"), []byte("argus.model_credential/v1\x00"+f.e.String()+"\x00"+revision.String()))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.Queries.CreateAIModelCredential(ctx, db.CreateAIModelCredentialParams{ID: uuid.New(), ModelRevisionID: revision, EnterpriseID: f.e, Provider: envelope.Provider, KeyID: envelope.KeyID, KeyVersion: int32(envelope.KeyVersion), WrappedDek: envelope.WrappedDEK, WrapNonce: envelope.WrapNonce, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, ValueHash: envelope.ValueHash}); err != nil {
				t.Fatal(err)
			}
			factory := toolruntime.CompositeFactory{Providers: []toolruntime.Provider{inferenceBudgetFixtureTools{invoked: &invoked}}}
			principal, err := (conversation.Service{Store: f.store}).ToolPrincipal(ctx, f.e, f.u, f.c)
			if err != nil {
				t.Fatal(err)
			}
			set, err := factory.Build(ctx, principal)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, _ := json.Marshal(set.Snapshot)
			f.exec("UPDATE runs SET tool_snapshot=$2,tool_snapshot_hash=$3 WHERE id=$1", f.r, snapshot, set.Snapshot.Hash())
			if _, err = conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "user_message", ActorType: "user", ActorID: f.u.String(), Payload: map[string]any{"content": "Read only what is necessary and provide a bounded truthful conclusion."}, Classification: "internal"}); err != nil {
				t.Fatal(err)
			}
			// Seed already-persisted inference records to exercise worker recovery,
			// not simulated provider successes in the current execution.
			zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
			for i := 0; i < tc.used; i++ {
				step, e := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: int32(i + 1), StepType: "model_call", Status: "succeeded"})
				if e != nil {
					t.Fatal(e)
				}
				call, e := f.store.Queries.CreateModelCall(ctx, db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: bytes.Repeat([]byte{0x42}, 32), InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
				if e != nil {
					t.Fatal(e)
				}
				f.exec("UPDATE model_calls SET status='succeeded',dispatched_at=now(),completed_at=now() WHERE id=$1", call.ID)
			}
			if tc.pending {
				actions := resource.PendingActionService{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}, Key: bytes.Repeat([]byte{6}, 32)}
				_, err = actions.Prepare(ctx, f.u.String(), f.e, resource.PrepareActionInput{RunID: uuid.NullUUID{UUID: f.r, Valid: true}, ActionType: "host.update", Title: "Pending controlled action", Summary: "No execution without user confirmation", Risk: "write", ResourceType: "host", AuthorizationVersion: 1, Preview: map[string]any{}, ImmutablePlan: map[string]any{"controlled": true}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit"}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
			}
			loop := Loop{Store: f.store, Tools: factory, Models: modelservice.Service{Store: f.store, Keyring: keyring}, EndpointPolicy: modelprovider.PublicEndpointPolicy{Resolver: usageFixtureResolver{}}}
			if err = loop.Handle(ctx, f.task("agent", "bounded-inference-recovery")); err != nil {
				t.Fatal(err)
			}
			actual := f.run()
			if actual.Status != tc.status || int(calls.Load()) != tc.wantCalls || int(invoked.Load()) != tc.wantTools {
				t.Fatalf("unexpected terminal state or dispatch count: status=%s calls=%d tools=%d", actual.Status, calls.Load(), invoked.Load())
			}
			if f.count("SELECT count(*) FROM model_calls WHERE run_id=$1 AND call_kind='inference'", f.r) != 24 {
				t.Fatal("durable inference budget changed")
			}
			if tc.status == "failed" && actual.ErrorCode.String != "RUN_STEP_LIMIT_REACHED" {
				t.Fatal("budget rejection reason changed")
			}
			if f.count("SELECT count(*) FROM tool_calls WHERE run_id=$1", f.r) != tc.wantTools {
				t.Fatal("final response dispatched or persisted a forbidden tool call")
			}
			if tc.pending && f.count("SELECT count(*) FROM user_confirmations WHERE enterprise_id=$1", f.e) != 0 {
				t.Fatal("budget implicitly confirmed an action")
			}
		})
	}
}
