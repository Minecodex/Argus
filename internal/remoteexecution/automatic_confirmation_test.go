package remoteexecution

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func autoWorkflow(f *fixture) action.Service {
	return action.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
}

func TestStopWinsBeforeAutomaticActionDispatch(t *testing.T) {
	f := newFixture(t)
	f.exec("UPDATE conversations SET access_mode='full_access' WHERE id=$1", f.c)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("printf must-not-run"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := autoWorkflow(f)
	if err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
		t.Fatal(err)
	}
	execution, err := f.store.Queries.GetExecutionByAction(t.Context(), db.GetExecutionByActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE runs SET status='cancelled' WHERE id=$1", f.r)
	payload, _ := json.Marshal(action.ExecutionTask{ExecutionID: execution.ID, EnterpriseID: f.e})
	executor := action.Executor{Store: f.store, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
	if err = executor.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	result, err := f.store.Queries.GetExecution(t.Context(), db.GetExecutionParams{ID: execution.ID, EnterpriseID: f.e})
	if err != nil || result.Status != "cancelled" {
		t.Fatalf("stopped action dispatched: %v %s", err, result.Status)
	}
	if f.transport.batches.Load() != 0 {
		t.Fatal("stopped command reached the transport")
	}
}
func fixtureRun(t *testing.T, f *fixture) db.Run {
	t.Helper()
	run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: f.r, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	return run
}
func TestConnectApprovalPolicyAllFourModeCombinations(t *testing.T) {
	for _, mode := range []string{"execute", "plan"} {
		for _, access := range []string{"request_approval", "full_access"} {
			t.Run(mode+"/"+access, func(t *testing.T) {
				f := newFixture(t)
				f.exec("UPDATE conversations SET mode=$2,access_mode=$3 WHERE id=$1", f.c, mode, access)
				v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("printf fixture"))
				if err != nil {
					t.Fatal(err)
				}
				workflow := autoWorkflow(f)
				if err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
					t.Fatal(err)
				}
				row, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
				if err != nil {
					t.Fatal(err)
				}
				expected := "awaiting_confirmation"
				if access == "full_access" {
					expected = "ready"
				}
				if row.Status != expected || row.ConfirmationMode != access || !row.ConfirmationPolicyVersion.Valid {
					t.Fatalf("policy/state %s %s", row.ConfirmationMode, row.Status)
				}
				if err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
					t.Fatal(err)
				}
				var confirmations, executions int
				_ = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM user_confirmations WHERE enterprise_id=$1 AND source='access_mode_policy'", f.e).Scan(&confirmations)
				_ = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM executions WHERE enterprise_id=$1", f.e).Scan(&executions)
				n := 0
				if access == "full_access" {
					n = 1
				}
				if confirmations != n || executions != n {
					t.Fatal("automatic confirmation was duplicated or request mode executed")
				}
			})
		}
	}
}
func TestAccessSwitchDoesNotRetroactivelyAuthorizePendingCommands(t *testing.T) {
	f := newFixture(t)
	old, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("printf old"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE conversations SET access_mode='full_access',version=version+1 WHERE id=$1", f.c)
	next, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("printf new"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE conversations SET access_mode='request_approval',version=version+1 WHERE id=$1", f.c)
	workflow := autoWorkflow(f)
	if err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ ref, status string }{{old.ActionRef, "awaiting_confirmation"}, {next.ActionRef, "ready"}} {
		r, e := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: test.ref, EnterpriseID: f.e})
		if e != nil || r.Status != test.status {
			t.Fatalf("switch changed frozen policy: %v %s", e, r.Status)
		}
	}
}
func TestFullAccessKeepsMFAEnterpriseApprovalAndRevocation(t *testing.T) {
	for _, boundary := range []string{"mfa", "enterprise_approval", "revocation", "historic_fixed_plan"} {
		t.Run(boundary, func(t *testing.T) {
			f := newFixture(t)
			f.exec("UPDATE conversations SET access_mode='full_access' WHERE id=$1", f.c)
			kind := "connect"
			if boundary == "historic_fixed_plan" {
				kind = "plan"
			}
			v, err := f.service.Prepare(t.Context(), f.call(), kind, f.input("printf boundary"))
			if err != nil {
				t.Fatal(err)
			}
			workflow := autoWorkflow(f)
			switch boundary {
			case "mfa":
				f.exec("UPDATE pending_actions SET risk='critical' WHERE action_ref=$1", v.ActionRef)
			case "enterprise_approval":
				_, err = workflow.CreatePolicy(t.Context(), f.u.String(), f.e, action.PolicyInput{Name: "Require independent approval", Enabled: true, ToolIDs: []string{}, ResourceTypes: []string{}, Risks: []string{"write"}, MinimumApprovers: 1, SeparationOfDuty: true, ApproverRoleIDs: []uuid.UUID{f.role}}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
			case "revocation":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e)
			}
			err = workflow.AutoConfirmRun(t.Context(), fixtureRun(t, f))
			if boundary == "revocation" {
				if err == nil {
					t.Fatal("revoked target was auto-confirmed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
			if err != nil {
				t.Fatal(err)
			}
			expected := "awaiting_confirmation"
			if boundary == "enterprise_approval" {
				expected = "awaiting_approval"
			}
			if r.Status != expected {
				t.Fatalf("bypassed %s: %s", boundary, r.Status)
			}
			if boundary == "mfa" {
				_, err = workflow.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, v.ActionRef, uuid.NewString())
				if !errors.Is(err, action.ErrStepUpRequired) {
					t.Fatalf("MFA boundary changed: %v", err)
				}
			}
		})
	}
}
