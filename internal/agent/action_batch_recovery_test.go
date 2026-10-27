package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresActionBatchRecoveryClosesBothPreviewCrashWindows(t *testing.T) {
	for _, window := range []string{"domain_committed", "result_committed"} {
		t.Run(window, func(t *testing.T) {
			f := newRecoveryFixture(t)
			task := f.task("agent", "owned action batch crash")
			ctx, cancel := withTaskLease(t.Context(), f.store, task)
			defer cancel()
			actions := resource.PendingActionService{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}, Key: bytes.Repeat([]byte{6}, 32)}
			reads, writes, fault := 0, 0, window == "domain_committed"
			definition := func(name, source string, read bool) toolruntime.Definition {
				return toolruntime.Definition{Model: modelprovider.Tool{Name: name, Schema: map[string]any{"type": "object"}}, Source: source, ReadOnly: read, Version: "owned-1"}
			}
			set, err := toolruntime.NewSet(toolruntime.Snapshot{}, []toolruntime.Tool{
				{Definition: definition("preview", "argus", true), Invoke: func(callCtx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
					action, err := actions.Prepare(callCtx, f.u.String(), f.e, resource.PrepareActionInput{ActionType: "host.update", Title: "Recovered batch", Summary: "Wait for confirmation", Risk: "write", ResourceType: "host", AuthorizationVersion: 1, Preview: map[string]any{}, ImmutablePlan: map[string]any{}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit", RunID: uuid.NullUUID{UUID: f.r, Valid: true}}, call.ID.String())
					if err != nil {
						return toolruntime.Result{}, err
					}
					if fault {
						fault = false
						cancel()
					}
					return toolruntime.Result{ActionRef: action.ActionRef, Data: map[string]any{"action_ref": action.ActionRef}}, nil
				}},
				{Definition: definition("read_after", "argus", true), Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
					reads++
					return toolruntime.Result{Data: map[string]any{"observed": true}}, nil
				}},
				{Definition: definition("write_after", "external_mcp", false), Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
					writes++
					return toolruntime.Result{}, nil
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			step, modelCall, _ := argumentModelResponse(t, f, ctx, 1, `{}`)
			scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
			if err != nil {
				t.Fatal(err)
			}
			// Persist the same complete native group as executeToolCalls, then
			// stop in either real transaction window around the first ToolResult.
			var calls []db.ToolCall
			var declared []modelprovider.ToolCall
			for i, name := range []string{"preview", "read_after", "write_after"} {
				input := []byte(`{}`)
				digest := sha256.Sum256(input)
				source := "argus"
				if i == 2 {
					source = "external_mcp"
				}
				id := uuid.New()
				record, err := f.store.Queries.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: id, CallID: id.String(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ToolID: name, Input: input, InputHash: digest[:], Source: source, ModelCallID: uuid.NullUUID{UUID: modelCall.ID, Valid: true}, CallSequence: int32(i), AuthorizationScope: scope})
				if err != nil {
					t.Fatal(err)
				}
				calls = append(calls, record)
				declared = append(declared, modelprovider.ToolCall{ID: id.String(), Name: name, Arguments: `{}`})
			}
			if err = fencedTransaction(ctx, f.store, func(q *db.Queries) error {
				_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "assistant_message", ActorType: "model", Payload: map[string]any{"content": "", "tool_calls": declared}, Classification: "internal"})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			loop := Loop{Store: f.store}
			p := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c, AuthorizationVersion: 1}
			_, err = loop.executePersistedTool(ctx, f.run(), set, p, false, calls[0])
			if (err != nil) != (window == "domain_committed") {
				t.Fatal("fault did not reach the intended durable window", err)
			}
			cancel()
			f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", task.ID)
			recovery, stop := withTaskLease(t.Context(), f.store, f.task("agent", "recover owned action batch"))
			defer stop()
			if recovered, err := loop.recoverToolCalls(recovery, f.run(), set, p, false); err != nil || !recovered {
				t.Fatal("action batch recovery failed", err)
			}
			if reads != 1 || writes != 0 || f.run().Status != "waiting_input" {
				t.Fatal("recovered batch stranded the Run or performed a write", reads, writes, f.run().Status)
			}
			for _, query := range []string{`SELECT count(*) FROM tool_calls WHERE run_id=$1 AND status IN ('prepared','dispatched')`, `SELECT count(*) FROM user_confirmations WHERE pending_action_id IN (SELECT id FROM pending_actions WHERE run_id=$1)`} {
				if n := f.count(query, f.r); n != 0 {
					t.Fatal("recovery left unfinished work or implicitly confirmed it", n)
				}
			}
			if n := f.count(`SELECT count(*) FROM pending_actions WHERE run_id=$1`, f.r); n != 1 {
				t.Fatal("replayed preview duplicated its action", n)
			}
			if n := f.count(`SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='tool_call_result'`, f.r); n != 3 {
				t.Fatal("recovered native messages are unpaired", n)
			}
			if n := f.count(`SELECT count(*) FROM tool_calls WHERE run_id=$1 AND tool_id='write_after' AND status='failed' AND error_code='TOOL_READ_ONLY_REQUIRED' AND dispatched_at IS NULL`, f.r); n != 1 {
				t.Fatal("recovery crossed the external write dispatch boundary")
			}
			if recovered, err := loop.recoverToolCalls(recovery, f.run(), set, p, false); err != nil || recovered || reads != 1 {
				t.Fatal("completed batch was replayed", err)
			}
		})
	}
}
