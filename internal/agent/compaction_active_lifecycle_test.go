//go:build m4e2e

package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestHardActiveTurnCompactionResumesWithPairedHistoryAndUserConstraints(t *testing.T) {
	f := newRecoveryFixture(t)
	f.exec(`UPDATE ai_models SET capabilities='{"output_token_parameter":"max_tokens","reasoning_effort":"low"}' WHERE id=$1`, f.m)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid summary request")
			w.WriteHeader(400)
			return
		}
		if len(body["tools"]) > 0 {
			t.Error("compaction must not dispatch tools")
		}
		if len(body["max_tokens"]) == 0 || len(body["max_completion_tokens"]) != 0 {
			t.Error("compaction did not use the revision's frozen budget parameter")
		}
		if string(body["reasoning_effort"]) != `"low"` {
			t.Error("compaction lost the immutable reasoning profile")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Prior tool groups completed; retain original user constraints and current server execution facts.\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":123,\"completion_tokens\":17,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	endpoint := strings.Replace(server.URL, "127.0.0.1", "argus-replay-model", 1)
	revision := uuid.New()
	f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT $2,id,enterprise_id,revision,$3,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m, revision, endpoint)
	keyPath := filepath.Join(t.TempDir(), "test-keyring.json")
	keyData, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}})
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := keyring.Encrypt([]byte("compaction-fixture-only"), []byte("argus.model_credential/v1\x00"+f.e.String()+"\x00"+revision.String()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Queries.CreateAIModelCredential(t.Context(), db.CreateAIModelCredentialParams{ID: uuid.New(), ModelRevisionID: revision, EnterpriseID: f.e, Provider: envelope.Provider, KeyID: envelope.KeyID, KeyVersion: int32(envelope.KeyVersion), WrappedDek: envelope.WrappedDEK, WrapNonce: envelope.WrapNonce, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, ValueHash: envelope.ValueHash})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE runs SET status='waiting_system',stop_reason='context_compaction',tool_snapshot='{"version":"argus.model_tool_set/v1","tools":[]}' WHERE id=$1`, f.r)
	scope, err := presentation.Scope(t.Context(), f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(kind string, payload map[string]any) {
		if kind == "assistant_message" || kind == "tool_call_result" {
			payload["authorization_scope"] = scope
		}
		_, err := conversation.AppendEvent(t.Context(), f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: kind, ActorType: "user", ActorID: f.u.String(), Payload: payload, Classification: "internal"})
		if err != nil {
			t.Fatal(err)
		}
	}
	goal := "Only inspect the owned fixture; keep the backup and never replay a completed modification."
	appendEvent("user_message", map[string]any{"content": goal})
	for n := 0; n < 8; n++ {
		id := fmt.Sprintf("call-%d", n)
		appendEvent("assistant_message", map[string]any{"tool_calls": []modelprovider.ToolCall{{ID: id, Name: "read", Arguments: `{}`}}})
		appendEvent("tool_call_result", map[string]any{"tool_call_id": id, "projection": map[string]any{"data": strings.Repeat("observed fixture result ", 450)}})
	}
	originalEvents := f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c)
	compactor := Compactor{Store: f.store, Models: modelservice.Service{Store: f.store, Keyring: keyring}, EndpointPolicy: modelprovider.PublicEndpointPolicy{Resolver: usageFixtureResolver{}}}
	if fits, err := compactor.currentContextFits(t.Context(), f.run()); err != nil || fits {
		t.Fatalf("oversized active turn did not reach hard limit: fits=%v err=%v", fits, err)
	}
	lastThrough := int64(0)
	for attempt := 0; attempt < 5; attempt++ {
		if err := compactor.Handle(t.Context(), f.task("compaction", "hard_limit")); err != nil {
			t.Fatal(err)
		}
		if f.run().Status != "pending" {
			t.Fatalf("hard compaction stranded the Run: %s", f.run().Status)
		}
		snapshot, err := f.store.Queries.GetActiveConversationSnapshot(t.Context(), db.GetActiveConversationSnapshotParams{ConversationID: f.c, EnterpriseID: f.e})
		if err != nil || snapshot.SourceThroughSequence <= lastThrough {
			t.Fatalf("compaction did not advance: through=%d err=%v", snapshot.SourceThroughSequence, err)
		}
		lastThrough = snapshot.SourceThroughSequence
		if fits, err := compactor.currentContextFits(t.Context(), f.run()); err != nil {
			t.Fatal(err)
		} else if fits {
			break
		}
		if attempt == 4 {
			t.Fatal("bounded history compaction did not recover the actual request budget")
		}
		f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", f.e)
		f.exec("UPDATE runs SET status='waiting_system',stop_reason='context_compaction' WHERE id=$1", f.r)
	}
	if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1 AND event_type<>'context_compacted'", f.c) != originalEvents {
		t.Fatal("original ledger events were deleted or rewritten")
	}
	if f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND queue='agent' AND status='pending'", f.r) == 0 {
		t.Fatal("no continuation task was persisted")
	}
	messages, _, _, _, err := (Loop{Store: f.store}).messages(t.Context(), f.run())
	if err != nil {
		t.Fatal(err)
	}
	retained := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == goal {
			retained++
		}
	}
	if retained != 1 {
		t.Fatalf("original user goal survived %d times", retained)
	}
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		if _, err := (modelprovider.Provider{Protocol: protocol}).RequestBytes(modelprovider.Request{Model: "fixture", Messages: messages, MaxTokens: 1024}); err != nil {
			t.Fatal(err)
		}
	}
}
