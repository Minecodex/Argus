package remoteexecution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func frozenPlan(t *testing.T, f *fixture) (uuid.UUID, CommandManifest) {
	t.Helper()
	call := f.call()
	planID := uuid.New()
	stepID := uuid.New()
	f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) SELECT $1,$2,$3,coalesce(max(sequence),0)+1,'tool_call','succeeded' FROM run_steps WHERE run_id=$2", stepID, f.r, f.e)
	f.exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status,authorization_scope) VALUES($1,$2,$3,$4,$5,'tool.invoke','{}',$6,'succeeded','fixture')", call.ID, call.ID.String(), f.e, f.r, stepID, bytes.Repeat([]byte{1}, 32))
	markdown := "# Owned repair\n\n```bash argus:backup\nprintf backup\n```\n\n```bash argus:verify\nprintf verify\n```\n"
	bindings := []CommandBinding{{BlockID: "backup", Description: "Back up fixture", Target: Target{ResourceType: "host", ResourceID: f.h}, Interpreter: "bash"}, {BlockID: "verify", Description: "Verify fixture", Target: Target{ResourceType: "host", ResourceID: f.h}, Interpreter: "bash"}}
	m, err := f.service.FreezePlanCommands(t.Context(), call, planID, "Owned repair", markdown, bindings)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(map[string]any{"title": "Owned repair", "markdown": markdown, "execution": m})
	_, err = f.store.Queries.CreateConversationInteraction(t.Context(), db.CreateConversationInteractionParams{ID: planID, EnterpriseID: f.e, ConversationID: f.c, OwnerUserID: f.u, RunID: f.r, ToolCallID: call.ID, Kind: "plan", FamilyID: planID, Revision: 1, Content: content, ContentHash: Digest(m), AuthorizationVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i, step := range m.Spec.Steps {
		spec := m.Spec
		spec.Steps = []Step{step}
		raw, _ := json.Marshal(spec)
		_, err = f.store.Queries.CreatePlanCommandAuthorization(t.Context(), db.CreatePlanCommandAuthorizationParams{ID: step.ID, EnterpriseID: f.e, ConversationID: f.c, OwnerUserID: f.u, PlanID: planID, Sequence: int32(i + 1), BlockID: m.Blocks[i], Spec: raw, SpecHash: Digest(spec)})
		if err != nil {
			t.Fatal(err)
		}
	}
	return planID, m
}
func activateFrozenPlan(f *fixture, id uuid.UUID) {
	f.exec("UPDATE runs SET status='succeeded' WHERE id=$1", f.r)
	f.r = uuid.New()
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status,approved_plan_id) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running',$6)", f.r, f.c, f.e, f.u, f.m, id)
	f.exec("UPDATE conversation_interactions SET status='confirmed',implementation_run_id=$2 WHERE id=$1", id, f.r)
	f.exec("UPDATE plan_command_authorizations SET implementation_run_id=$2 WHERE plan_id=$1", id, f.r)
}
func executePlanCommand(t *testing.T, f *fixture, v View) {
	t.Helper()
	workflow := autoWorkflow(f)
	if err := workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
		t.Fatal(err)
	}
	e, err := f.store.Queries.GetExecutionByAction(t.Context(), db.GetExecutionByActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(action.ExecutionTask{ExecutionID: e.ID, EnterpriseID: f.e})
	executor := action.Executor{Store: f.store, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
	if err = executor.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
}

func TestPlanCommandsRequireConsentAndRunOnceInOrder(t *testing.T) {
	f := newFixture(t)
	id, m := frozenPlan(t, f)
	if _, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[0].ID); err == nil {
		t.Fatal("unconfirmed plan executed")
	}
	activateFrozenPlan(f, id)
	if _, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[1].ID); err == nil {
		t.Fatal("later step skipped predecessor")
	}
	var wg sync.WaitGroup
	views := make(chan View, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[0].ID)
			views <- v
			failures <- e
		}()
	}
	wg.Wait()
	close(views)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first View
	for v := range views {
		if first.ID != uuid.Nil && first.ID != v.ID {
			t.Fatal("duplicate step created multiple executions")
		}
		first = v
	}
	if first.ConfirmationRequired {
		t.Fatal("approved step demanded ordinary confirmation")
	}
	if first.Spec.Steps[0].Script != "printf backup\n" {
		t.Fatal("approved script changed")
	}
	executePlanCommand(t, f, first)
	replay, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[0].ID)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("step was replayed: %v", err)
	}
	second, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	executePlanCommand(t, f, second)
	if f.transport.batches.Load() != 2 {
		t.Fatalf("executed %d commands", f.transport.batches.Load())
	}
	var confirmations int
	err = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM user_confirmations WHERE enterprise_id=$1 AND source='plan_approval'", f.e).Scan(&confirmations)
	if err != nil || confirmations != 2 {
		t.Fatalf("missing plan consent provenance: %v %d", err, confirmations)
	}
	ordinary, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("printf extra"))
	if err != nil {
		t.Fatal(err)
	}
	if err = autoWorkflow(f).AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
		t.Fatal(err)
	}
	row, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: ordinary.ActionRef, EnterpriseID: f.e})
	if err != nil || row.Status != "awaiting_confirmation" {
		t.Fatal("plan authorized an additional command")
	}
}

func TestPlanCommandConsentCannotCrossBoundaries(t *testing.T) {
	for _, scenario := range []string{"other_run", "other_conversation", "revocation", "identity_changed", "target_changed", "context_changed", "stopped", "unconfirmed", "failed_predecessor", "unknown_predecessor"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			id, m := frozenPlan(t, f)
			activateFrozenPlan(f, id)
			call := f.call()
			command := m.Spec.Steps[0].ID
			switch scenario {
			case "other_run":
				call.RunID = uuid.New()
			case "other_conversation":
				call.Principal.ConversationID = uuid.New()
			case "revocation":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e)
			case "identity_changed":
				f.exec("UPDATE remote_execution_identities SET version=version+1 WHERE enterprise_id=$1", f.e)
			case "target_changed":
				f.exec("UPDATE hosts SET resource_version=resource_version+1 WHERE id=$1", f.h)
			case "context_changed":
				f.exec("UPDATE remote_execution_sessions SET context_revision=context_revision+1 WHERE enterprise_id=$1", f.e)
			case "stopped":
				f.exec("UPDATE runs SET status='cancelled' WHERE id=$1", f.r)
			case "unconfirmed":
				f.exec("UPDATE conversation_interactions SET status='superseded' WHERE id=$1", id)
			case "failed_predecessor", "unknown_predecessor":
				v, err := f.service.PrepareApproved(t.Context(), call, command)
				if err != nil {
					t.Fatal(err)
				}
				state := "failed"
				if scenario == "unknown_predecessor" {
					state = "result_unknown"
				}
				f.exec("UPDATE remote_execution_requests SET status=$2 WHERE id=$1", v.ID, state)
				command = m.Spec.Steps[1].ID
				call = f.call()
			}
			if _, err := f.service.PrepareApproved(t.Context(), call, command); err == nil {
				t.Fatalf("crossed %s", scenario)
			}
			if f.transport.batches.Load() != 0 {
				t.Fatal("denied step reached transport")
			}
		})
	}
}

func TestApprovedConnectRejectsReplacementArguments(t *testing.T) {
	f := newFixture(t)
	id, m := frozenPlan(t, f)
	activateFrozenPlan(f, id)
	call := f.call()
	tools, err := f.service.BuildTools(t.Context(), call.Principal)
	if err != nil {
		t.Fatal(err)
	}
	call.Arguments = map[string]any{"approved_step_id": m.Spec.Steps[0].ID.String(), "title": "replace", "steps": []any{map[string]any{"script": "printf changed"}}}
	if _, err = tools.Tools[0].Invoke(t.Context(), call); err == nil {
		t.Fatal("mixed a grant with new arguments")
	}
	if f.transport.batches.Load() != 0 {
		t.Fatal(fmt.Sprint("unexpected dispatch"))
	}
	call.Arguments = map[string]any{"approved_step_id": m.Spec.Steps[0].ID.String()}
	result, err := tools.Tools[0].Invoke(t.Context(), call)
	if err != nil || result.Display == nil || result.Display.Mode != "action" || result.Display.ActionRef != result.ActionRef || result.ActionRef == "" {
		t.Fatalf("connect lost its authoritative timeline correlation: %v %#v", err, result.Display)
	}
}
