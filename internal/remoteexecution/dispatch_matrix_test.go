package remoteexecution

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestCurrentAuthorityAndContextAreRevalidatedAtConfirmationAndDispatch(t *testing.T) {
	cases := []struct {
		name   string
		change func(*fixture, View)
	}{
		{"credential_version_changed", func(f *fixture, _ View) {
			f.exec("UPDATE credentials SET version=version+1 WHERE enterprise_id=$1", f.e)
		}},
		{"credential_disabled", func(f *fixture, _ View) {
			f.exec("UPDATE credentials SET status='disabled' WHERE enterprise_id=$1", f.e)
		}},
		{"user_disabled", func(f *fixture, _ View) { f.exec("UPDATE enterprise_users SET status='disabled' WHERE id=$1", f.u) }},
		{"enterprise_suspended", func(f *fixture, _ View) { f.exec("UPDATE enterprises SET status='suspended' WHERE id=$1", f.e) }},
		{"department_disabled", func(f *fixture, _ View) { f.exec("UPDATE departments SET status='disabled' WHERE id=$1", f.d) }},
		{"execute_permission_revoked", func(f *fixture, _ View) {
			f.exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='remote_execution.execute'", f.role)
		}},
		{"resource_grant_revoked", func(f *fixture, _ View) { f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e) }},
		{"resource_deleted", func(f *fixture, _ View) { f.exec("UPDATE hosts SET status='deleted' WHERE id=$1", f.h) }},
		{"resource_version_changed", func(f *fixture, _ View) {
			f.exec("UPDATE hosts SET name='changed',resource_version=resource_version+1 WHERE id=$1", f.h)
		}},
		{"identity_changed", func(f *fixture, v View) {
			f.exec("UPDATE remote_execution_identities SET version=version+1 WHERE id=$1", v.Spec.Steps[0].Identity.ID)
		}},
		{"context_changed", func(f *fixture, v View) {
			f.exec("UPDATE remote_execution_sessions SET context_revision=context_revision+1 WHERE id=$1", v.Spec.Steps[0].SessionID)
		}},
		{"generation_replaced", func(f *fixture, v View) {
			f.exec("UPDATE remote_execution_sessions SET generation=$2 WHERE id=$1", v.Spec.Steps[0].SessionID, uuid.New())
		}},
	}
	for _, tc := range cases {
		for _, phase := range []string{"confirmation", "dispatch"} {
			t.Run(tc.name+"/"+phase, func(t *testing.T) {
				f := newFixture(t)
				if strings.HasPrefix(tc.name, "credential_") {
					secret, err := f.store.Queries.CreateSecret(t.Context(), db.CreateSecretParams{ID: uuid.New(), EnterpriseID: f.e, Name: "rotation-fixture", Type: "ssh_password", CreatedBy: f.u})
					if err != nil {
						t.Fatal(err)
					}
					credential, err := f.store.Queries.CreateCredential(t.Context(), db.CreateCredentialParams{ID: uuid.New(), EnterpriseID: f.e, Name: "rotation-fixture", Protocol: "ssh", Username: "fixture-account", SecretID: secret.ID})
					if err != nil {
						t.Fatal(err)
					}
					f.exec("UPDATE remote_execution_identities SET credential_id=$2,credential_version=$3 WHERE enterprise_id=$1", f.e, credential.ID, credential.Version)
				}
				view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("approved effect"))
				if err != nil {
					t.Fatal(err)
				}
				if phase == "confirmation" {
					tc.change(f, view)
					workflow := action.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
					if _, err = workflow.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, view.ActionRef, uuid.NewString()); err == nil {
						t.Fatal("invalid confirmation accepted")
					}
				} else {
					command := queuedFixtureCommand(f, view, 1)
					if err = ValidateQueuedCommand(t.Context(), f.store.Queries, command); err != nil {
						t.Fatalf("valid queued command rejected: %v", err)
					}
					tc.change(f, view)
					if err = ValidateQueuedCommand(t.Context(), f.store.Queries, command); err == nil {
						t.Fatal("changed authority reached dispatch")
					}
				}
				if f.transport.batches.Load() != 0 {
					t.Fatal("revalidation test executed remote effects")
				}
			})
		}
	}
}

func queuedFixtureCommand(f *fixture, v View, sequence int) db.ConnectorCommand {
	f.t.Helper()
	f.approve(v)
	f.exec("UPDATE remote_execution_requests SET status='running' WHERE id=$1", v.ID)
	f.exec("UPDATE remote_execution_sessions SET active_request_id=$2,status='active' WHERE id=$1", v.Spec.Steps[sequence-1].SessionID, v.ID)
	f.exec("UPDATE remote_execution_steps SET status='preparing' WHERE id=$1", v.Spec.Steps[sequence-1].ID)
	req, err := f.store.Queries.GetRemoteRequest(f.t.Context(), db.GetRemoteRequestParams{ID: v.ID, EnterpriseID: f.e})
	if err != nil {
		f.t.Fatal(err)
	}
	binding := batchBinding(req, v.Spec.Steps[sequence-1], sequence, 0)
	raw, err := protojson.Marshal(binding)
	if err != nil {
		f.t.Fatal(err)
	}
	raw, err = resource.CanonicalJSON(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return db.ConnectorCommand{ID: uuid.New(), EnterpriseID: f.e, ConnectorID: f.connector, ConnectionEpoch: 1, CommandType: "remote_execution_batch", Payload: raw, PayloadHash: digest[:]}
}

func TestDispatchRejectsWrongConnectorAndOutOfOrderPlan(t *testing.T) {
	t.Run("connector", func(t *testing.T) {
		f := newFixture(t)
		v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("approved effect"))
		if err != nil {
			t.Fatal(err)
		}
		c := queuedFixtureCommand(f, v, 1)
		c.ConnectorID = uuid.New()
		if err = ValidateQueuedCommand(t.Context(), f.store.Queries, c); err == nil {
			t.Fatal("dispatch allowed a different Connector")
		}
	})
	t.Run("order", func(t *testing.T) {
		f := newFixture(t)
		v, err := f.service.Prepare(t.Context(), f.call(), "plan", f.input("first", "second"))
		if err != nil {
			t.Fatal(err)
		}
		c := queuedFixtureCommand(f, v, 2)
		if err = ValidateQueuedCommand(t.Context(), f.store.Queries, c); err == nil {
			t.Fatal("second step bypassed the unfinished first step")
		}
	})
}

func TestExpiredPreviewCannotCreateExecution(t *testing.T) {
	f := newFixture(t)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "connect", f.input("must not execute"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE pending_actions SET expires_at=now()-interval '1 second' WHERE action_ref=$1", v.ActionRef)
	expired, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil || expired.Status != "expired" {
		t.Fatalf("expired preview still available: %v %#v", err, expired)
	}
	workflow := action.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Resources: resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}}
	if _, err = workflow.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, v.ActionRef, uuid.NewString()); err == nil {
		t.Fatal("expired preview accepted confirmation")
	}
	var executions int
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM executions WHERE remote_execution_id=$1", v.ID).Scan(&executions); err != nil || executions != 0 {
		t.Fatal("expired preview created an execution", err)
	}
}
