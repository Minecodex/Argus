package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestConversationRecoveryFiltersRevokedSummariesAndPreservesToolPairs(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, user, model, convo := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'P5 context',$2,'UTC')", enterprise, "p5-"+enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'P5 context')", user, enterprise, department, "p5-"+user.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, enterprise)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'P5 context',$4)", convo, enterprise, user, model)
	runs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range runs {
		exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'succeeded')", id, convo, enterprise, user, model)
	}
	scope, err := presentation.Scope(ctx, store, enterprise, user)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(run uuid.UUID, kind string, payload map[string]any) {
		t.Helper()
		_, err := conversation.AppendEvent(ctx, store.Queries, conversation.EventInput{EnterpriseID: enterprise, ConversationID: convo, RunID: uuid.NullUUID{UUID: run, Valid: true}, Type: kind, ActorType: "model", Payload: payload, Classification: "internal"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, run := range runs[:2] {
		appendEvent(run, "user_message", map[string]any{"content": "Read host state"})
		appendEvent(run, "assistant_message", map[string]any{"content": "restricted-model-output", "authorization_scope": scope, "tool_calls": []modelprovider.ToolCall{{ID: run.String(), Name: "tool_invoke", Arguments: `{"name":"restricted-argument"}`}}})
		appendEvent(run, "tool_call_result", map[string]any{"tool_call_id": run.String(), "authorization_scope": scope, "projection": map[string]any{"summary": "restricted-result", "page": i}})
		appendEvent(run, "assistant_message", map[string]any{"content": "restricted-final-answer", "authorization_scope": scope})
	}
	appendEvent(runs[2], "user_message", map[string]any{"content": "Continue the latest conversation"})
	checkpoint, _ := json.Marshal(scopedCheckpoint{AuthorizationScope: scope, Execution: json.RawMessage(`{}`)})
	_, err = store.Queries.CreateContextSnapshot(ctx, db.CreateContextSnapshotParams{ID: uuid.New(), EnterpriseID: enterprise, ConversationID: convo, RunID: runs[0], Revision: 1, SourceFromSequence: 1, SourceThroughSequence: 4, FirstKeptSequence: 5, TypedCheckpoint: checkpoint, NarrativeSummary: "restricted-earlier-summary", CompactionModelID: model, CompactionModelRevision: 1, PromptVersion: "test", SourceHash: make([]byte, 32), SnapshotHash: make([]byte, 32), Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.Queries.GetRun(ctx, db.GetRunParams{ID: runs[2], EnterpriseID: enterprise})
	if err != nil {
		t.Fatal(err)
	}
	loop := Loop{Store: store}
	before, current, _, _, err := loop.messages(ctx, run)
	if err != nil || current != "Continue the latest conversation" {
		t.Fatalf("cross-Run restore failed: %v", err)
	}
	encoded, _ := json.Marshal(before)
	if !strings.Contains(string(encoded), "restricted-earlier-summary") || !strings.Contains(string(encoded), runs[1].String()) || strings.Contains(string(encoded), runs[0].String()) {
		t.Fatal("summary and incremental tail were not restored")
	}
	exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", user)
	after, _, newScope, _, err := loop.messages(ctx, run)
	if err != nil || newScope == scope {
		t.Fatalf("authorization change was not observed: %v", err)
	}
	encoded, _ = json.Marshal(after)
	if strings.Contains(string(encoded), "restricted-") {
		t.Fatal("revoked model output or summary reached the restored context")
	}
	calls, results := 0, 0
	for _, message := range after {
		calls += len(message.ToolCalls)
		if message.Role == "tool" {
			results++
		}
	}
	if calls != 2 || results != 2 {
		t.Fatal("filtering broke native tool pairs")
	}
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		if _, err := (modelprovider.Provider{Protocol: protocol}).RequestBytes(modelprovider.Request{Model: "model", Messages: after, MaxTokens: 1024}); err != nil {
			t.Fatalf("filtered history cannot be sent to %s: %v", protocol, err)
		}
	}
	first, err := store.Queries.ClaimContextRevision(ctx, db.ClaimContextRevisionParams{ID: convo, EnterpriseID: enterprise, ContextRevision: 0})
	if err != nil || first != 1 {
		t.Fatalf("context revision claim failed: %v", err)
	}
	if _, err := store.Queries.ClaimContextRevision(ctx, db.ClaimContextRevisionParams{ID: convo, EnterpriseID: enterprise, ContextRevision: 0}); err == nil {
		t.Fatal("stale compactor overwrote the conversation revision")
	}
}
