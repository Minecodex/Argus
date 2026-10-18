package remoteexecution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestApprovedPlanStillRequiresCurrentMFAApprovalAndPermissions(t *testing.T) {
	for _, boundary := range []string{"mfa", "enterprise_approval", "revocation", "stop"} {
		t.Run(boundary, func(t *testing.T) {
			f := newFixture(t)
			id, m := frozenPlan(t, f)
			activateFrozenPlan(f, id)
			v, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			workflow := autoWorkflow(f)
			switch boundary {
			case "mfa":
				f.exec("UPDATE pending_actions SET risk='critical' WHERE action_ref=$1", v.ActionRef)
			case "enterprise_approval":
				_, err = workflow.CreatePolicy(t.Context(), f.u.String(), f.e, action.PolicyInput{Name: "Independent review", Enabled: true, ToolIDs: []string{}, ResourceTypes: []string{}, Risks: []string{"write"}, MinimumApprovers: 1, SeparationOfDuty: true, ApproverRoleIDs: []uuid.UUID{f.role}}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
			case "revocation":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e)
			}
			err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f))
			if boundary == "revocation" {
				if err == nil {
					t.Fatal("revocation bypassed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "stop" {
				e, err := f.store.Queries.GetExecutionByAction(t.Context(), db.GetExecutionByActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
				if err != nil {
					t.Fatal(err)
				}
				f.exec("UPDATE runs SET status='cancelled' WHERE id=$1", f.r)
				payload, _ := json.Marshal(action.ExecutionTask{ExecutionID: e.ID, EnterpriseID: f.e})
				executor := action.Executor{Store: f.store, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
				if err = executor.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
					t.Fatal(err)
				}
				ended, err := f.store.Queries.GetExecution(t.Context(), db.GetExecutionParams{ID: e.ID, EnterpriseID: f.e})
				if err != nil || ended.Status != "cancelled" {
					t.Fatalf("stopped grant dispatched: %v %s", err, ended.Status)
				}
			} else {
				row, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
				expected := "awaiting_confirmation"
				if boundary == "enterprise_approval" {
					expected = "awaiting_approval"
				}
				if err != nil || row.Status != expected {
					t.Fatalf("bypassed %s: %v %s", boundary, err, row.Status)
				}
			}
			if f.transport.batches.Load() != 0 {
				t.Fatal("blocked command reached transport")
			}
		})
	}
}

type planWorkspace struct {
	recursiveWorkspace
	freezes int
	file    File
}

func (w *planWorkspace) FreezeRemoteInputs(context.Context, toolruntime.Principal, uuid.UUID, uuid.UUID) ([]File, error) {
	w.freezes++
	return []File{w.file}, nil
}
func (w *planWorkspace) ValidateRemoteInputs(context.Context, toolruntime.Principal, []File) error {
	if w.denied {
		return Error("REMOTE_RESOURCE_FORBIDDEN")
	}
	return nil
}

func TestPlanRetainsSealedInputsAndRechecksTheirSource(t *testing.T) {
	f := newFixture(t)
	w := &planWorkspace{file: File{ID: uuid.New(), Path: "config.txt", Size: 8, Hash: hashBytes([]byte("original"))}}
	f.service.Files = w
	id, m := frozenPlan(t, f)
	activateFrozenPlan(f, id)
	w.file.Hash = hashBytes([]byte("changed!"))
	v, err := f.service.PrepareApproved(t.Context(), f.call(), m.Spec.Steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if w.freezes != 1 || len(v.Spec.Inputs) != 1 || v.Spec.Inputs[0].Hash != m.Spec.Inputs[0].Hash {
		t.Fatal("execution resealed mutable workspace input")
	}
	files, err := f.store.Queries.ListRemoteFiles(t.Context(), db.ListRemoteFilesParams{RequestID: v.ID, EnterpriseID: f.e, Direction: "input"})
	if err != nil || len(files) != 1 || files[0].ObjectKey != inputObjectKey(f.e, id, "config.txt", m.Spec.Inputs[0].Hash) {
		t.Fatalf("lost plan snapshot: %v %#v", err, files)
	}
	w.denied = true
	if err = autoWorkflow(f).AutoConfirmRun(t.Context(), fixtureRun(t, f)); err == nil {
		t.Fatal("revoked source was authorized")
	}
	if f.transport.batches.Load() != 0 {
		t.Fatal("revoked input was dispatched")
	}
}

func TestNewPlanCanReplaceExpiredUnusedContextButCannotReuseOldConsent(t *testing.T) {
	f := newFixture(t)
	id, original := frozenPlan(t, f)
	step := original.Spec.Steps[0]
	f.exec("UPDATE remote_execution_sessions SET status='reclaimed' WHERE id=$1", step.SessionID)
	f.exec("UPDATE conversation_interactions SET status='superseded' WHERE id=$1", id)
	_, replacement := frozenPlan(t, f)
	if replacement.Spec.Steps[0].Generation == step.Generation {
		t.Fatal("new preview reused expired context")
	}
	activateFrozenPlan(f, id)
	if _, err := f.service.PrepareApproved(t.Context(), f.call(), step.ID); err == nil {
		t.Fatal("old consent survived replacement")
	}
	if f.transport.batches.Load() != 0 {
		t.Fatal("preview ran a command")
	}
}
