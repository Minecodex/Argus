package agent

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestPostgresActionPreviewCompletesReadTailAndRejectsExternalWriteBeforeWaiting(t *testing.T) {
	f := newRecoveryFixture(t)
	task := f.task("agent", "owned preview/read batch regression")
	ctx, cancel := withTaskLease(t.Context(), f.store, task)
	defer cancel()
	actions := resource.PendingActionService{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}, Key: bytes.Repeat([]byte{6}, 32)}
	reads, writes := 0, 0
	definition := func(name, source string, read bool) toolruntime.Definition {
		return toolruntime.Definition{Model: modelprovider.Tool{Name: name, Schema: map[string]any{"type": "object", "additionalProperties": false}}, Source: source, ReadOnly: read, Version: "owned-1"}
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, []toolruntime.Tool{
		{Definition: definition("preview", "argus", true), Invoke: func(callCtx context.Context, _ toolruntime.Invocation) (toolruntime.Result, error) {
			action, err := actions.Prepare(callCtx, f.u.String(), f.e, resource.PrepareActionInput{ActionType: "host.update", Title: "Batch preview", Summary: "No write until confirmation", Risk: "write", ResourceType: "host", AuthorizationVersion: 1, Preview: map[string]any{}, ImmutablePlan: map[string]any{"controlled": true}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit", RunID: uuid.NullUUID{UUID: f.r, Valid: true}}, uuid.NewString())
			if err != nil {
				return toolruntime.Result{}, err
			}
			return toolruntime.Result{ActionRef: action.ActionRef, Data: map[string]any{"action_ref": action.ActionRef}}, nil
		}},
		{Definition: definition("read_after", "argus", true), Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			reads++
			return toolruntime.Result{Data: map[string]any{"observed": true}}, nil
		}},
		{Definition: definition("write_after", "external_mcp", false), Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			writes++
			return toolruntime.Result{Data: map[string]any{"wrote": true}}, nil
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
	p := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c, AuthorizationVersion: 1}
	results, err := (Loop{Store: f.store}).executeToolCalls(ctx, f.run(), step, modelCall, set, p, false, "", scope, []*pendingToolCall{{ID: "preview", Name: "preview", Arguments: `{}`}, {ID: "read", Name: "read_after", Arguments: `{}`, Index: 1}, {ID: "write", Name: "write_after", Arguments: `{}`, Index: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || reads != 1 || writes != 0 {
		t.Fatalf("preview stranded its batch or dispatched an unconfirmed external write: results=%d reads=%d writes=%d", len(results), reads, writes)
	}
	if n := f.count(`SELECT count(*) FROM tool_calls WHERE run_id=$1 AND status IN ('prepared','dispatched')`, f.r); n != 0 {
		t.Fatal("unfinished native message group blocks waiting state", n)
	}
	if n := f.count(`SELECT count(*) FROM tool_calls WHERE run_id=$1 AND tool_id='write_after' AND status='failed' AND error_code='TOOL_READ_ONLY_REQUIRED' AND dispatched_at IS NULL`, f.r); n != 1 {
		t.Fatal("external write was not refused before dispatch")
	}
	if n := f.count(`SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='tool_call_result'`, f.r); n != 3 {
		t.Fatal("tool results were not paired exactly once", n)
	}
	current, paused, err := (Loop{Store: f.store}).reconcileActions(ctx, f.run())
	if err != nil || !paused || current.Status != "waiting_input" {
		t.Fatal("complete preview batch did not reach human confirmation", current.Status, err)
	}
	if n := f.count(`SELECT count(*) FROM user_confirmations WHERE enterprise_id=$1`, f.e); n != 0 {
		t.Fatal("batch implicitly confirmed an action")
	}
}
