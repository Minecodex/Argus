package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func argumentModelResponse(t *testing.T, f *recoveryFixture, ctx context.Context, sequence int, arguments string) (db.RunStep, db.ModelCall, *pendingToolCall) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "model-call", "function": map[string]any{"name": "inspect", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\ndata: [DONE]\n\n", raw)
	}))
	defer server.Close()
	var usage modelprovider.TokenUsage
	var pending *pendingToolCall
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: server.URL, Client: server.Client()}
	if err := provider.Stream(ctx, modelprovider.Request{Model: "arguments", MaxTokens: 32}, func(event modelprovider.Event) error {
		usage.Observe(event)
		if event.Type == "tool_call_done" {
			pending = &pendingToolCall{ID: event.ToolCallID, Name: event.ToolName, Arguments: event.Arguments}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if pending == nil || !usage.Complete() {
		t.Fatal("complete response/usage missing")
	}
	step, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, Sequence: int32(sequence), StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	digest := sha256.Sum256([]byte("argument-fixture"))
	call, err := f.store.Queries.CreateModelCall(ctx, db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: digest[:], InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
	if err != nil {
		t.Fatal(err)
	}
	if err = (Loop{Store: f.store}).finishModelCall(ctx, call, db.AiModelRevision{InputPricePerMillion: zero, OutputPricePerMillion: zero}, usage, time.Millisecond, "tool_calls", "succeeded", "", zero); err != nil {
		t.Fatal(err)
	}
	return step, call, pending
}

func argumentTestSet(t *testing.T, invoked *int) *toolruntime.Set {
	t.Helper()
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, []toolruntime.Tool{{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "inspect", Schema: map[string]any{"type": "object", "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}}}, Source: "argus", ReadOnly: true}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
		*invoked++
		return toolruntime.Result{Data: map[string]any{"ok": true}}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestMalformedArgumentsArePersistedRejectedAndRecoverable(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx, cancel := withTaskLease(t.Context(), f.store, f.task("agent", "argument validation"))
	defer cancel()
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	principal := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c, AuthorizationVersion: 1}
	invoked := 0
	set := argumentTestSet(t, &invoked)
	loop := Loop{Store: f.store}
	bad := []string{`{"private-payload":`, "", `null`, `[]`, `"text"`}
	for index, args := range bad {
		step, call, pending := argumentModelResponse(t, f, ctx, index+1, args)
		results, err := loop.executeToolCalls(ctx, f.run(), step, call, set, principal, false, "", scope, []*pendingToolCall{pending})
		if err != nil || len(results) != 1 || !strings.Contains(results[0].message, "TOOL_INPUT_INVALID") || !strings.Contains(results[0].message, "/arguments") || strings.Contains(results[0].message, "private-payload") || invoked != 0 {
			t.Fatalf("invalid arguments were not safely rejected: %v", err)
		}
		if f.count("SELECT count(*) FROM tool_calls WHERE model_call_id=$1 AND status='failed' AND error_code='TOOL_INPUT_INVALID' AND dispatched_at IS NULL", call.ID) != 1 {
			t.Fatal("invalid input crossed dispatch boundary")
		}
		if f.count("SELECT count(*) FROM model_calls WHERE id=$1 AND status='succeeded' AND input_tokens=12 AND output_tokens=4 AND input_usage_source='provider' AND output_usage_source='provider'", call.ID) != 1 {
			t.Fatal("argument validation lost provider accounting")
		}
	}
	// Crash after the prepared record and assistant event commit, before result.
	step, call, pending := argumentModelResponse(t, f, ctx, len(bad)+1, `{"private-payload":`)
	id := uuid.NewSHA1(call.ID, []byte(pending.ID))
	encoded := storedToolInput(pending.Arguments)
	hash := sha256.Sum256(encoded)
	err = f.store.InTx(ctx, func(q *db.Queries) error {
		_, err := q.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: id, CallID: pending.ID, EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ToolID: pending.Name, Source: "argus", Input: encoded, InputHash: hash[:], ModelCallID: uuid.NullUUID{UUID: call.ID, Valid: true}, AuthorizationScope: scope})
		if err != nil {
			return err
		}
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "assistant_message", ActorType: "model", ActorID: f.m.String(), Payload: map[string]any{"content": "", "authorization_scope": scope, "tool_calls": []modelprovider.ToolCall{{ID: id.String(), Name: pending.Name, Arguments: pending.Arguments}}}, Classification: "internal"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh := Loop{Store: f.store}
	if recovered, err := fresh.recoverToolCalls(ctx, f.run(), set, principal, false); err != nil || !recovered {
		t.Fatal("restart lost invalid-call outcome", err)
	}
	if invoked != 0 || f.count("SELECT count(*) FROM tool_calls WHERE id=$1 AND status='failed' AND dispatched_at IS NULL", id) != 1 {
		t.Fatal("restart invoked malformed input")
	}
	// Paired raw history stays encodable; a later corrected object runs once.
	messages, input, _, source, err := fresh.messages(ctx, f.run())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := fresh.context(ctx, f.run(), db.AiModelRevision{ContextWindowTokens: 65536, MaxOutputTokens: 1024}, messages, input, set.Models(), source)
	if err != nil {
		t.Fatal("context assembly rejected correction history", err)
	}
	if _, err = (modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: "https://model.example.test"}).RequestBytes(modelprovider.Request{Model: "fixture", Messages: projection.Messages}); err != nil {
		t.Fatal("correction history broken", err)
	}
	step, call, pending = argumentModelResponse(t, f, ctx, len(bad)+2, `{"value":"ok"}`)
	if _, err = fresh.executeToolCalls(ctx, f.run(), step, call, set, principal, false, "", scope, []*pendingToolCall{pending}); err != nil || invoked != 1 {
		t.Fatal("corrected call failed", err)
	}
}

func TestDispatchedMalformedArgumentsRemainUnknownWithoutReplay(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx, cancel := withTaskLease(t.Context(), f.store, f.task("agent", "unknown arguments"))
	defer cancel()
	step, call, pending := argumentModelResponse(t, f, ctx, 1, `{"value":`)
	encoded := storedToolInput(pending.Arguments)
	hash := sha256.Sum256(encoded)
	record, err := f.store.Queries.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: uuid.New(), CallID: pending.ID, EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ToolID: pending.Name, Source: "argus", Input: encoded, InputHash: hash[:], ModelCallID: uuid.NullUUID{UUID: call.ID, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE tool_calls SET status='dispatched',dispatched_at=now() WHERE id=$1", record.ID)
	record.Status = "dispatched"
	invoked := 0
	set := argumentTestSet(t, &invoked)
	result, err := (Loop{Store: f.store}).executePersistedTool(ctx, f.run(), set, toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c}, false, record)
	if err != nil || !result.unknown || invoked != 0 {
		t.Fatal("ambiguous dispatched input was replayed or called unexecuted", err)
	}
}
