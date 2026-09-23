package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type recoveryExecutor struct {
	actions resource.PendingActionService
	commits int
}

func TestCancellingOneRecoveredPreviewPreservesOtherActions(t *testing.T) {
	f := newRecoveryFixture(t)
	loop := Loop{Store: f.store}
	actions := resource.PendingActionService{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}, Key: bytes.Repeat([]byte{6}, 32)}
	var refs []string
	for i := 0; i < 2; i++ {
		value, err := actions.Prepare(t.Context(), f.u.String(), f.e, resource.PrepareActionInput{ActionType: "host.update", Title: "Two previews", Summary: "Preview", Risk: "write", ResourceType: "host", AuthorizationVersion: 1, Preview: map[string]any{}, ImmutablePlan: map[string]any{"index": i}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit", RunID: uuid.NullUUID{UUID: f.r, Valid: true}}, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, value.ActionRef)
	}
	if _, paused, err := loop.reconcileActions(t.Context(), f.run()); err != nil || !paused {
		t.Fatalf("wait: %v", err)
	}
	// Cancel the last announced action first: the other confirmation stays live.
	if _, err := actions.Cancel(t.Context(), f.u.String(), f.e, refs[1], uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if f.run().Status != "waiting_input" {
		t.Fatal("cancelling one action terminated the other")
	}
	if _, paused, err := loop.reconcileActions(t.Context(), f.run()); err != nil || !paused {
		t.Fatalf("other action was not retained: %v", err)
	}
	if _, err := actions.Cancel(t.Context(), f.u.String(), f.e, refs[0], uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if f.run().Status != "cancelled" {
		t.Fatal("last cancellation stranded the Run")
	}
	if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='pending_action_created'", f.r); n != 2 {
		t.Fatalf("action events=%d", n)
	}
	if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='cancelled'", f.r); n != 1 {
		t.Fatalf("terminal events=%d", n)
	}
}

func (e *recoveryExecutor) RevalidatePendingAction(_ context.Context, _ *db.Queries, a db.PendingAction, _ json.RawMessage) ([]byte, error) {
	return a.ImpactHash, nil
}
func (e *recoveryExecutor) CommitPendingAction(context.Context, *db.Queries, db.PendingAction, json.RawMessage) (resource.ActionCommitResult, error) {
	e.commits++
	return resource.ActionCommitResult{Summary: "committed once"}, nil
}
func (e *recoveryExecutor) ExecutePendingAction(ctx context.Context, q *db.Queries, a db.PendingAction) (resource.ActionCommitResult, error) {
	return e.actions.ExecuteReady(ctx, q, a, e.RevalidatePendingAction, e.CommitPendingAction)
}

func TestPreviewRecoveryRestoresHostControlAndDeterministicVerification(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	loop := Loop{Store: f.store}
	idempotency := postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}
	actions := resource.PendingActionService{Store: f.store, Idempotency: idempotency, Key: bytes.Repeat([]byte{6}, 32)}
	preview, err := actions.Prepare(ctx, f.u.String(), f.e, resource.PrepareActionInput{ActionType: "host.update", Title: "Recovery", Summary: "Preview", Risk: "write", ResourceType: "host", AuthorizationVersion: 1,
		Preview: map[string]any{"summary": "public"}, ImmutablePlan: map[string]any{"operation": "test"}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit", RunID: uuid.NullUUID{UUID: f.r, Valid: true}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	step, call := uuid.New(), uuid.New()
	f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'model_call','running')", step, f.r, f.e)
	f.exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'tool.invoke','argus','{}',decode(repeat('00',32),'hex'),'dispatched')", call, call.String(), f.e, f.r, step)
	if _, paused, err := loop.reconcileActions(ctx, f.run()); err != nil || paused {
		t.Fatalf("unfinished batch was abandoned: %v", err)
	}
	// First crash window: Preview committed, ToolResult not yet persisted.
	var record db.ToolCall
	record.ID, record.StepID, record.EnterpriseID, record.RunID, record.ToolID = call, step, f.e, f.r, "tool.invoke"
	if _, err := loop.persistToolOutcome(ctx, f.run(), record, toolruntime.Result{ActionRef: preview.ActionRef, ToolID: "host.update.preview", Data: map[string]any{"action_ref": preview.ActionRef}}, nil); err != nil {
		t.Fatal(err)
	}
	// Second window: the result committed, but no wait event or model summary.
	// The intentionally unavailable model must not block control restoration.
	task := f.task("agent", "tools_recovered")
	for repeat := 0; repeat < 3; repeat++ {
		if err := loop.Handle(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if f.run().Status != "waiting_input" {
		t.Fatalf("restored Run=%s", f.run().Status)
	}
	if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='pending_action_created'", f.r); n != 1 {
		t.Fatalf("host events=%d", n)
	}
	if n := f.count("SELECT count(*) FROM model_calls WHERE run_id=$1", f.r); n != 0 {
		t.Fatalf("control recovery required model calls=%d", n)
	}
	executor := &recoveryExecutor{actions: actions}
	service := action.Service{Store: f.store, Idempotency: idempotency, Resources: executor}
	key := uuid.NewString()
	var confirmation action.Confirmation
	for repeat := 0; repeat < 2; repeat++ {
		confirmation, err = service.Confirm(ctx, f.u.String(), key, f.e, 1, false, preview.ActionRef, key)
		if err != nil {
			t.Fatal(err)
		}
	}
	if confirmation.Execution == nil {
		t.Fatal("single confirmation did not create execution")
	}
	body, _ := json.Marshal(action.ExecutionTask{ExecutionID: confirmation.Execution.ID, EnterpriseID: f.e})
	commit := action.Executor{Store: f.store, Resources: executor}
	for repeat := 0; repeat < 2; repeat++ {
		if err := commit.Handle(ctx, runtime.Task{RuntimeTask: db.RuntimeTask{Payload: body}}); err != nil {
			t.Fatal(err)
		}
	}
	if executor.commits != 1 {
		t.Fatalf("commit count=%d", executor.commits)
	}
	current, paused, err := loop.reconcileActions(ctx, f.run())
	if err != nil || paused || !current.VerificationOnly {
		t.Fatalf("verification not restored: paused=%v readonly=%v err=%v", paused, current.VerificationOnly, err)
	}
	public, err := loop.executionVerification(ctx, current)
	if err != nil || !strings.Contains(public, confirmation.Execution.ExecutionRef) || !strings.Contains(public, "committed once") {
		t.Fatalf("missing execution facts: %v", err)
	}
	if strings.Contains(public, "immutable_plan") || strings.Contains(public, "commit_token") {
		t.Fatal("private commit data reached verification")
	}
	// A task restored for another reason still uses the durable read-only flag.
	writeStep, writeCall := uuid.New(), uuid.New()
	f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,2,'model_call','running')", writeStep, f.r, f.e)
	f.exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'mcp_write','external_mcp','{}',decode(repeat('00',32),'hex'),'prepared')", writeCall, writeCall.String(), f.e, f.r, writeStep)
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, []toolruntime.Tool{{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "mcp_write", Schema: map[string]any{"type": "object"}}, Source: "external_mcp", Version: "1"}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
		t.Fatal("verification dispatched an external write")
		return toolruntime.Result{}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.executePersistedTool(ctx, current, set, toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c}, false, db.ToolCall{ID: writeCall, StepID: writeStep, RunID: f.r, EnterpriseID: f.e, ToolID: "mcp_write", Source: "external_mcp", Status: "prepared", Input: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if n := f.count("SELECT count(*) FROM tool_calls WHERE id=$1 AND status='failed' AND error_code='TOOL_READ_ONLY_REQUIRED' AND dispatched_at IS NULL", writeCall); n != 1 {
		t.Fatal("recovered verification did not reject the write before dispatch")
	}
	if err := loop.finishRun(ctx, current, "succeeded", "stop", ""); err != nil {
		t.Fatal(err)
	}
	if err := loop.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	if n := f.count("SELECT count(*) FROM user_confirmations WHERE pending_action_id=$1", preview.ID); n != 1 {
		t.Fatalf("confirmations=%d", n)
	}
	if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='succeeded'", f.r); n != 1 {
		t.Fatalf("terminal events=%d", n)
	}
}
