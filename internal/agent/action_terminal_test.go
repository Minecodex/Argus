package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func terminalActionFixture(t *testing.T, f *recoveryFixture) (db.PendingAction, action.Service, *recoveryExecutor) {
	t.Helper()
	idempotency := postgres.Idempotency{Key: bytes.Repeat([]byte{5}, 32)}
	actions := resource.PendingActionService{Store: f.store, Idempotency: idempotency, Key: bytes.Repeat([]byte{6}, 32)}
	preview, err := actions.Prepare(t.Context(), f.u.String(), f.e, resource.PrepareActionInput{ActionType: "host.update", Title: "Terminal action", Summary: "Preview", Risk: "write", ResourceType: "host", AuthorizationVersion: 1, Preview: map[string]any{}, ImmutablePlan: map[string]any{"operation": "test"}, ResourceScopeSnapshot: map[string]any{}, CommitHandler: "test.commit", RunID: uuid.NullUUID{UUID: f.r, Valid: true}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	executor := &recoveryExecutor{actions: actions}
	if _, paused, err := (Loop{Store: f.store}).reconcileActions(t.Context(), f.run()); err != nil || !paused {
		t.Fatalf("prepare wait: %v", err)
	}
	return preview, action.Service{Store: f.store, Idempotency: idempotency, Resources: executor}, executor
}

func consumeActionContinuation(t *testing.T, f *recoveryFixture) {
	t.Helper()
	var id uuid.UUID
	var payload []byte
	err := f.store.Pool.QueryRow(t.Context(), "UPDATE runtime_tasks SET status='running',lease_owner='terminal-test',lease_until=now()+interval '1 minute',fence_token=fence_token+1,attempt=attempt+1 WHERE id=(SELECT id FROM runtime_tasks WHERE run_id=$1 AND queue='agent' AND status='pending' ORDER BY created_at LIMIT 1) RETURNING id,payload", f.r).Scan(&id, &payload)
	if err != nil {
		t.Fatalf("missing durable continuation: %v", err)
	}
	task := runtime.Task{RuntimeTask: db.RuntimeTask{ID: id, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Payload: payload, LeaseOwner: pgtype.Text{String: "terminal-test", Valid: true}, FenceToken: 1}}
	if err := (Loop{Store: f.store}).Handle(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", id)
}

func assertActionRunTerminated(t *testing.T, f *recoveryFixture) {
	t.Helper()
	if !terminalRun(f.run().Status) {
		t.Fatalf("Run still active: %s", f.run().Status)
	}
	if _, err := f.store.Queries.GetActiveRunForConversation(t.Context(), db.GetActiveRunForConversationParams{ConversationID: f.c, EnterpriseID: f.e}); err != pgx.ErrNoRows {
		t.Fatalf("next message remains blocked: %v", err)
	}
	if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status' IN ('failed','cancelled','succeeded')", f.r); n != 1 {
		t.Fatalf("terminal event count=%d", n)
	}
	if n := f.count("SELECT count(*) FROM model_calls WHERE run_id=$1", f.r); n != 0 {
		t.Fatal("error finalization invoked the model")
	}
}

func TestRejectedApprovalTerminatesRunWithoutAnotherModel(t *testing.T) {
	f := newRecoveryFixture(t)
	approver, role, policy := uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Approver')", approver, f.e, f.d, "approver-"+approver.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name,builtin) VALUES($1,$2,'Approver',false)", role, f.e)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES(gen_random_uuid(),$1,'user',$2,$3)", f.e, approver, role)
	f.exec("INSERT INTO approval_policies(id,enterprise_id,name,enabled,tool_ids,risks,resource_types,minimum_approvers,separation_of_duty,approver_role_ids,expires_after_seconds) VALUES($1,$2,'Approval',true,ARRAY['test.commit'],ARRAY['write'],ARRAY['host'],1,true,ARRAY[$3::uuid],300)", policy, f.e, role)
	preview, service, _ := terminalActionFixture(t, f)
	confirmation, err := service.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, preview.ActionRef, uuid.NewString())
	if err != nil || confirmation.ApprovalRequest == nil {
		t.Fatalf("confirm: %v", err)
	}
	consumeActionContinuation(t, f)
	if f.run().Status != "waiting_approval" {
		t.Fatal("approval state was not propagated")
	}
	key := uuid.NewString()
	for repeat := 0; repeat < 2; repeat++ {
		if _, err := service.Decide(t.Context(), approver.String(), f.e, confirmation.ApprovalRequest.ID, "rejected", "not approved", key); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND queue='agent' AND status='pending'", f.r); n != 1 {
		t.Fatalf("continuations=%d", n)
	}
	consumeActionContinuation(t, f)
	assertActionRunTerminated(t, f)
	if f.run().ErrorCode.String != "APPROVAL_REJECTED" {
		t.Fatalf("error=%s", f.run().ErrorCode.String)
	}
}

func TestExecutionInvalidationWakesRunAfterRejectingCommit(t *testing.T) {
	f := newRecoveryFixture(t)
	preview, service, executor := terminalActionFixture(t, f)
	confirmation, err := service.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, preview.ActionRef, uuid.NewString())
	if err != nil || confirmation.Execution == nil {
		t.Fatalf("confirm: %v", err)
	}
	consumeActionContinuation(t, f)
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.u)
	payload, _ := json.Marshal(action.ExecutionTask{ExecutionID: confirmation.Execution.ID, EnterpriseID: f.e})
	for repeat := 0; repeat < 2; repeat++ {
		if err := (action.Executor{Store: f.store, Resources: executor}).Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
			t.Fatal(err)
		}
	}
	if executor.commits != 0 {
		t.Fatal("invalidated action executed")
	}
	consumeActionContinuation(t, f)
	assertActionRunTerminated(t, f)
}

func TestExpiredPreviewIsRetiredWithoutDispatch(t *testing.T) {
	f := newRecoveryFixture(t)
	preview, _, executor := terminalActionFixture(t, f)
	f.exec("UPDATE pending_actions SET expires_at=now()-interval '1 second' WHERE id=$1", preview.ID)
	r := action.Reconciler{Executor: action.Executor{Store: f.store, Resources: executor}}
	for repeat := 0; repeat < 2; repeat++ {
		if err := r.ReconcileExpired(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	consumeActionContinuation(t, f)
	assertActionRunTerminated(t, f)
	if n := f.count("SELECT count(*) FROM pending_actions WHERE id=$1 AND status='expired'", preview.ID); n != 1 {
		t.Fatal("preview did not expire")
	}
}

func TestActionContinuationDoesNotRelyOnAnAlreadyRunningTask(t *testing.T) {
	f := newRecoveryFixture(t)
	preview, _, _ := terminalActionFixture(t, f)
	_ = f.task("agent", "older_state")
	for i := 0; i < 2; i++ {
		if err := f.store.InTx(t.Context(), func(q *db.Queries) error { return conversation.ScheduleActionRun(t.Context(), q, preview, false) }); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND queue='agent' AND status='pending'", f.r); n != 1 {
		t.Fatalf("missing or duplicate pending successor: %d", n)
	}
	// A cancelled terminal Run cannot be revived by a late callback.
	f.exec("UPDATE runs SET status='cancelled' WHERE id=$1", f.r)
	f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE run_id=$1", f.r)
	if err := f.store.InTx(context.Background(), func(q *db.Queries) error {
		return conversation.ScheduleActionRun(context.Background(), q, preview, true)
	}); err != nil {
		t.Fatal(err)
	}
	if f.run().VerificationOnly || f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND status='pending'", f.r) != 0 {
		t.Fatal("terminal Run was changed by callback")
	}
}
