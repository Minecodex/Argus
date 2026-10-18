package remoteexecution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type remoteNotifyFunc func(context.Context, uuid.UUID, int64)

func (fn remoteNotifyFunc) NotifyConnectorCommand(ctx context.Context, id uuid.UUID, epoch int64) {
	fn(ctx, id, epoch)
}

func TestConnectorRemoteControlLeaseUsesRegisteredAuditActor(t *testing.T) {
	f := newFixture(t)
	keyFile := filepath.Join(t.TempDir(), "keyring.json")
	data, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}})
	if err := os.WriteFile(keyFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	secrets := secret.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Keyring: keyring}
	record, err := secrets.Create(t.Context(), f.u.String(), f.e, secret.SecretInput{Name: "remote-lease", Type: "ssh_password", Value: "private-remote-lease-fixture"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	credential, err := secrets.CreateCredential(t.Context(), f.u.String(), f.e, secret.CredentialInput{Name: "remote-lease", Protocol: "ssh", Username: "fixture-account", SecretID: record.Secret.ID}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE remote_execution_identities SET credential_id=$2,credential_version=$3 WHERE enterprise_id=$1", f.e, credential.ID, credential.Version)
	f.exec("UPDATE connectors SET capabilities=ARRAY['remote_execution.v1'] WHERE id=$1", f.connector)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("approved"))
	if err != nil {
		t.Fatal(err)
	}
	_ = queuedFixtureCommand(f, v, 1)
	r, err := f.store.Queries.GetRemoteRequest(t.Context(), db.GetRemoteRequestParams{ID: v.ID, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	step := v.Spec.Steps[0]
	identity, err := f.store.Queries.GetRemoteIdentityByID(t.Context(), db.GetRemoteIdentityByIDParams{ID: step.Identity.ID, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	transport := ConnectorTransport{Store: f.store, Secrets: secrets, Notifier: remoteNotifyFunc(func(ctx context.Context, id uuid.UUID, _ int64) {
		if _, err := f.store.Pool.Exec(ctx, "UPDATE connector_commands SET status='succeeded',result='{}' WHERE connector_id=$1 AND command_type='remote_execution_control'", id); err != nil {
			t.Error(err)
		}
	})}
	_, err = transport.Call(t.Context(), identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "open", SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: v.ID.String(), Binding: batchBinding(r, step, 1, 0)}, "credential-control-test", 3*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	var actor, payload, recipient string
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT actor_type FROM audit_events WHERE enterprise_id=$1 AND action='credential.use' ORDER BY created_at DESC,id DESC LIMIT 1", f.e).Scan(&actor); err != nil || actor != "connector" {
		t.Fatal("credential use actor is not the authenticated recipient", err)
	}
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT c.payload::text,l.recipient_id FROM connector_commands c JOIN credential_leases l ON l.id=c.credential_lease_id WHERE c.enterprise_id=$1 AND c.command_type='remote_execution_control'", f.e).Scan(&payload, &recipient); err != nil || recipient != f.connector.String() || strings.Contains(payload, "private-remote-lease-fixture") {
		t.Fatal("credential lease escaped its private recipient boundary", err)
	}
}
