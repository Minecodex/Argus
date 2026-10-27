package connector

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresServerRejectsUndeliveredCommandsWithoutOpeningClientTransition(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	e, h, c := uuid.New(), uuid.New(), uuid.New()
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned undelivered commands',$2,'UTC')", e, e.String())
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx, err := store.Pool.Begin(cleanup)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(cleanup)
		for _, query := range []string{"DELETE FROM audit_chain_heads WHERE enterprise_id=$1", "DELETE FROM audit_events WHERE enterprise_id=$1", "DELETE FROM connector_commands WHERE enterprise_id=$1", "DELETE FROM connector_sessions WHERE enterprise_id=$1", "UPDATE hosts SET connector_id=NULL WHERE enterprise_id=$1", "DELETE FROM connectors WHERE enterprise_id=$1", "DELETE FROM hosts WHERE enterprise_id=$1", "DELETE FROM enterprises WHERE id=$1"} {
			if _, err = tx.Exec(cleanup, query, e); err != nil {
				t.Error(err)
				return
			}
		}
		if err = tx.Commit(cleanup); err != nil {
			t.Error(err)
		}
	})
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,'Owned dispatch host','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", h, e)
	exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at)VALUES($1,$2,'host','Owned dispatch Connector',$3,$4,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',1,now()+interval '1 hour')", c, e, h, c.String())
	exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", h, c)
	_, err = store.Queries.UpsertConnectorSession(ctx, db.UpsertConnectorSessionParams{ConnectorID: c, EnterpriseID: e, GatewayInstanceID: "owned", ConnectionEpoch: 1, Capabilities: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	identity := TrustedIdentity{EnterpriseID: e, ConnectorID: c}
	service := Service{Store: store}
	create := func() string {
		t.Helper()
		id := uuid.NewString()
		exec("INSERT INTO connector_commands(id,command_id,enterprise_id,connector_id,connection_epoch,operation_ref,command_type,payload_schema_version,payload,payload_hash,idempotency_key,expires_at)VALUES($1,$2,$3,$4,1,$2,'kubernetes_resource_query','fixture','{}',decode(repeat('00',32),'hex'),$2,now()+interval '1 minute')", uuid.New(), id, e, c)
		return id
	}
	first := create()
	command, err := store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: first, ConnectorID: c, ConnectionEpoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, prepareErr := (Gateway{}).typedCommand(ctx, command)
	code := dispatchPreparationFailureCode(prepareErr)
	if code != "CONNECTOR_COMMAND_SCHEMA_INVALID" {
		t.Fatal("malformed frame not classified before dispatch")
	}
	if _, err = service.TransitionCommand(ctx, identity, 1, first, "failed", nil, code); !errors.Is(err, ErrCommandState) {
		t.Fatal("Connector client can fail queued commands", err)
	}
	failed, err := service.rejectCommandBeforeDispatch(ctx, identity, 1, first, code)
	if err != nil || failed.Status != "failed" || failed.ErrorCode.String != code || !failed.CompletedAt.Valid {
		t.Fatal("undelivered command remained queued", err)
	}
	if _, err = service.rejectCommandBeforeDispatch(ctx, identity, 1, first, code); !errors.Is(err, ErrCommandState) {
		t.Fatal("duplicate rejection was accepted", err)
	}
	fenced := create()
	for _, actor := range []TrustedIdentity{{EnterpriseID: uuid.New(), ConnectorID: c}, {EnterpriseID: e, ConnectorID: uuid.New()}} {
		if _, err = service.rejectCommandBeforeDispatch(ctx, actor, 1, fenced, code); err == nil {
			t.Fatal("foreign identity rejected another command")
		}
	}
	if _, err = service.rejectCommandBeforeDispatch(ctx, identity, 2, fenced, code); !errors.Is(err, ErrConnectorFenced) {
		t.Fatal("stale epoch accepted", err)
	}
	current, err := store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: fenced, ConnectorID: c, ConnectionEpoch: 1})
	if err != nil || current.Status != "queued" {
		t.Fatal("foreign/fenced rejection changed a queued command", err)
	}
	exec("UPDATE connector_commands SET status='dispatched' WHERE command_id=$1", fenced)
	if _, err = service.rejectCommandBeforeDispatch(ctx, identity, 1, fenced, code); !errors.Is(err, ErrCommandState) {
		t.Fatal("server labelled delivered work undispatched", err)
	}
	var audits int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE enterprise_id=$1 AND action='connector.command.rejected_before_dispatch'", e).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("trusted rejection audit is missing or duplicated", audits, err)
	}
}

func TestPostgresCredentialDependencyFailuresStayRetryable(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := secret.Service{Store: store}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = credentials.PrepareMonitoringLeaseWithQueries(cancelled, store.Queries, "owned", uuid.New(), secret.MonitoringDelivery{CollectionID: uuid.New(), OperationRef: "owned", RecipientType: "connector", RecipientID: "owned", TTL: time.Minute})
	if !errors.Is(err, context.Canceled) || dispatchPreparationFailureCode(err) != "" {
		t.Fatal("temporary monitoring lookup failure became a permanent lease rejection", err)
	}
	blocker, err := store.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(t.Context(), "LOCK TABLE credential_leases IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(t.Context(), 350*time.Millisecond)
	_, err = credentials.FulfillLease(deadline, uuid.New(), uuid.New(), "connector", "owned")
	stop()
	if !errors.Is(err, context.DeadlineExceeded) || dispatchPreparationFailureCode(err) != "" {
		t.Fatal("blocked credential backend became a permanent rejection", err)
	}
	if err = blocker.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = credentials.FulfillLease(t.Context(), uuid.New(), uuid.New(), "connector", "owned")
	if !errors.Is(err, secret.ErrInvalidLease) || dispatchPreparationFailureCode(err) != "CREDENTIAL_UNAVAILABLE" {
		t.Fatal("missing credential was not a permanent scoped rejection", err)
	}
}
