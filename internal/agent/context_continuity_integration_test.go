package agent

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestCompactionRehydratesExactNativeContractWithoutDispatchOrRedaction(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "user_message", ActorType: "user", Payload: map[string]any{"content": "Keep this exact user condition"}, Classification: "internal"}); err != nil {
		t.Fatal(err)
	}
	step, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"category":"host","name":"inspect"}`)
	sum := sha256.Sum256(input)
	id := uuid.New()
	_, err = f.store.Queries.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: id, CallID: id.String(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ToolID: "tool.describe", Input: input, InputHash: sum[:], Source: "argus", AuthorizationScope: scope})
	if err != nil {
		t.Fatal(err)
	}
	schema := map[string]any{"category": "host", "name": "inspect", "version": "frozen/v1", "input_schema": map[string]any{
		"type": "object", "required": []string{"api_key"}, "properties": map[string]any{"api_key": map[string]any{"type": "string"}}}}
	body, _ := json.Marshal(schema)
	projection, _, err := encodeToolResultProjection("result_"+id.String(), id.String(), body, toolruntime.Result{Data: schema, DataIsSchema: true})
	if err != nil {
		t.Fatal(err)
	}
	event, err := conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "tool_call_result", ActorType: "service", Payload: map[string]any{
		"tool_id": "tool.describe", "tool_call_id": id.String(), "target_tool_id": "tool.describe", "source": "argus", "authorization_scope": scope, "status": "succeeded", "result_ref": "result_" + id.String(), "projection": json.RawMessage(projection)}, Classification: "internal"})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _ := json.Marshal(scopedCheckpoint{AuthorizationScope: scope, Execution: json.RawMessage(`{}`)})
	_, err = f.store.Queries.CreateContextSnapshot(ctx, db.CreateContextSnapshotParams{ID: uuid.New(), EnterpriseID: f.e, ConversationID: f.c, RunID: f.r, Revision: 1, SourceFromSequence: 1, SourceThroughSequence: event.Sequence, FirstKeptSequence: event.Sequence + 1,
		TypedCheckpoint: checkpoint, NarrativeSummary: "Inspect contract was read, some details omitted", CompactionModelID: f.m, CompactionModelRevision: 1, PromptVersion: "fixture", SourceHash: make([]byte, 32), SnapshotHash: make([]byte, 32), Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	before := f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c)
	loop := Loop{Store: f.store}
	messages, _, _, _, err := loop.messages(ctx, f.run())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID == id.String() {
			found = strings.Contains(m.Content, `"api_key"`) && strings.Contains(m.Content, `"required"`)
		}
	}
	if !found {
		t.Fatal("restored schema was missing or redacted")
	}
	if _, err = (modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions}).RequestBytes(modelprovider.Request{Model: "fixture", Messages: messages, MaxTokens: 1024}); err != nil {
		t.Fatal("restored tool pair is invalid", err)
	}
	if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c) != before {
		t.Fatal("metadata restoration created execution events")
	}
	revoked, err := loop.toolContextFacts(ctx, f.run(), event.Sequence, event.Sequence, "different-scope")
	if err != nil || len(revoked.Messages) != 0 {
		t.Fatal("contract survived a different authorization scope", err)
	}
}

func TestNativeContextFactsKeepLatestAuthorizedResultAtLedgerBoundary(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	step, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	appendResult := func(status, source string, version int, extra ...string) int64 {
		t.Helper()
		id := uuid.New()
		input := []byte(`{"category":"dashboard","name":"query.get","input":{"id":"job-one"}}`)
		sum := sha256.Sum256(input)
		_, e := f.store.Queries.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: id, CallID: id.String(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ToolID: "tool.invoke", Input: input, InputHash: sum[:], Source: source, AuthorizationScope: scope})
		if e != nil {
			t.Fatal(e)
		}
		payload := map[string]any{"tool_id": "tool.invoke", "tool_call_id": id.String(), "target_tool_id": "telemetry.dashboard.query.get", "source": source,
			"authorization_scope": scope, "status": "succeeded", "result_ref": "result_" + id.String(), "projection": map[string]any{"summary": map[string]any{"status": status}},
			"context_facts": map[string]any{"kind": "query_job", "id": "job-one", "status": status, "version": version, "files": []string{"/workspace/results/exact-full-file-id.json"}}}
		if len(extra) != 0 {
			payload["context_facts"].(map[string]any)["detail"] = extra[0]
		}
		event, e := conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "tool_call_result", ActorType: "service", Payload: payload, Classification: "internal"})
		if e != nil {
			t.Fatal(e)
		}
		return event.Sequence
	}
	queued := appendResult("queued", "argus", 1)
	complete := appendResult("complete", "argus", 2)
	spoof := appendResult("spoofed", "external_mcp", 99)
	loop := Loop{Store: f.store}
	check := func(through int64, currentScope, wantStatus string) {
		t.Helper()
		value, e := loop.toolContextFacts(ctx, f.run(), through, through, currentScope)
		if e != nil {
			t.Fatal(e)
		}
		rows := value.Facts["recent_results"].([]map[string]any)
		if wantStatus == "" {
			if len(rows) != 0 {
				t.Fatal("facts survived authorization scope change")
			}
			return
		}
		if len(rows) != 1 || rows[0]["facts"].(map[string]any)["status"] != wantStatus || len(value.Messages) != 0 {
			t.Fatalf("wrong latest result at %d: %#v", through, value)
		}
	}
	before := f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c)
	check(queued, scope, "queued")
	check(complete, scope, "complete")
	check(spoof, scope, "complete")
	check(spoof, "revoked-scope", "")
	if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c) != before {
		t.Fatal("context read created execution events")
	}
	oversized := appendResult("newer", "argus", 3, strings.Repeat("x", 13<<10))
	value, err := loop.toolContextFacts(ctx, f.run(), oversized, oversized, scope)
	if err != nil || len(value.Facts["recent_results"].([]map[string]any)) != 0 || value.Facts["complete"] != false {
		t.Fatalf("oversized latest state fell back to stale state: %#v, %v", value, err)
	}
}
