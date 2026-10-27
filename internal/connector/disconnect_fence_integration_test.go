package connector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresDisconnectProjectsAtomicallyBeforeNewSession(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	enterprise, host, scope, connector := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned disconnect race',$2,'UTC')", enterprise, enterprise.String())
	t.Logf("owned_disconnect_enterprise=%s", enterprise)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		tx, err := store.Pool.Begin(cleanup)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(cleanup)
		for _, query := range []string{
			"UPDATE hosts SET connector_id=NULL WHERE enterprise_id=$1",
			"UPDATE bastion_scopes SET active_connector_id=NULL,connector_host_id=NULL WHERE enterprise_id=$1",
			"DELETE FROM connector_sessions WHERE enterprise_id=$1",
			"DELETE FROM connectors WHERE enterprise_id=$1",
			"DELETE FROM hosts WHERE enterprise_id=$1",
			"DELETE FROM bastion_scopes WHERE enterprise_id=$1",
		} {
			if _, err = tx.Exec(cleanup, query, enterprise); err != nil {
				t.Error("owned disconnect fixture cleanup", err)
				return
			}
		}
		if _, err = tx.Exec(cleanup, "DELETE FROM enterprises WHERE id=$1 AND code=$2", enterprise, enterprise.String()); err != nil {
			t.Error(err)
			return
		}
		if err = tx.Commit(cleanup); err != nil {
			t.Error(err)
		}
	})
	exec("INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status,relay_status) VALUES($1,$2,'Owned race bastion','development',decode(repeat('00',32),'hex'),'command','active','ready')", scope, enterprise)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,bastion_scope_id,labels_hash,connection_status) VALUES($1,$2,'Owned race host','',0,'development','linux','bastion','direct',$3,decode(repeat('00',32),'hex'),'online')", host, enterprise, scope)
	exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at) VALUES($1,$2,'bastion','Owned race connector',$3,$4,$5,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',1,now()+interval '1 hour')", connector, enterprise, host, scope, connector.String())
	exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", host, connector)
	exec("UPDATE bastion_scopes SET connector_host_id=$2,active_connector_id=$3 WHERE id=$1", scope, host, connector)
	if _, err = store.Queries.UpsertConnectorSession(ctx, db.UpsertConnectorSessionParams{ConnectorID: connector, EnterpriseID: enterprise, GatewayInstanceID: "previous", ConnectionEpoch: 1, Capabilities: []string{}}); err != nil {
		t.Fatal(err)
	}
	blocker, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	var blockerPID int
	if err = blocker.QueryRow(ctx, "SELECT pg_backend_pid() FROM hosts WHERE id=$1 FOR UPDATE", host).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	disconnected := make(chan struct{})
	service := Service{Store: store}
	go func() {
		service.Disconnect(ctx, TrustedIdentity{EnterpriseID: enterprise, ConnectorID: connector}, 1)
		close(disconnected)
	}()
	var oldPID int
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = store.Pool.QueryRow(ctx, "SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%UPDATE hosts SET connection_status%' LIMIT 1", blockerPID).Scan(&oldPID)
		if err == nil {
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("old disconnect did not reach its blocked dependent projection")
		}
		time.Sleep(10 * time.Millisecond)
	}
	next, err := store.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	var nextPID int
	if err = next.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&nextPID); err != nil {
		t.Fatal(err)
	}
	opened := make(chan error, 1)
	go func() {
		transaction, err := next.Begin(ctx)
		if err != nil {
			opened <- err
			return
		}
		defer transaction.Rollback(context.Background())
		q := store.Queries.WithTx(transaction)
		_, err = q.AdvanceConnectorEpoch(ctx, db.AdvanceConnectorEpochParams{ID: connector, EnterpriseID: enterprise})
		if err == nil {
			_, err = q.RestoreBastionConnectorOnline(ctx, db.RestoreBastionConnectorOnlineParams{EnterpriseID: enterprise, ActiveConnectorID: uuid.NullUUID{UUID: connector, Valid: true}})
		}
		if err == nil {
			_, err = q.UpsertConnectorSession(ctx, db.UpsertConnectorSessionParams{ConnectorID: connector, EnterpriseID: enterprise, GatewayInstanceID: "next", ConnectionEpoch: 2, Capabilities: []string{}})
		}
		if err == nil {
			err = transaction.Commit(ctx)
		}
		opened <- err
	}()
	fenced := false
	nextFinished := false
	deadline = time.Now().Add(3 * time.Second)
	for !fenced && !nextFinished && time.Now().Before(deadline) {
		select {
		case err = <-opened:
			nextFinished = true
			if err != nil {
				t.Error(err)
			}
		default:
		}
		if err = store.Pool.QueryRow(ctx, "SELECT $2=ANY(pg_blocking_pids($1))", nextPID, oldPID).Scan(&fenced); err != nil {
			t.Fatal(err)
		}
		if !fenced && !nextFinished {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !fenced || nextFinished {
		t.Error("new connection advanced before the old disconnect committed its dependent state")
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal("disconnect did not finish")
	}
	if !nextFinished {
		select {
		case err = <-opened:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("new connection did not finish")
		}
	}
	var status, relay, connectorStatus string
	var epoch int64
	if err = store.Pool.QueryRow(ctx, "SELECT b.status,b.relay_status,c.status,c.connection_epoch FROM bastion_scopes b JOIN connectors c ON c.id=b.active_connector_id WHERE b.id=$1", scope).Scan(&status, &relay, &connectorStatus, &epoch); err != nil {
		t.Fatal(err)
	}
	if status != "active" || connectorStatus != "online" || epoch != 2 {
		t.Fatalf("old disconnect overwrote the new connection: %s/%s epoch%d relay%s", status, connectorStatus, epoch, relay)
	}
	session, err := store.Queries.GetConnectorSession(ctx, db.GetConnectorSessionParams{ConnectorID: connector, ConnectionEpoch: 2})
	if err != nil || session.GatewayInstanceID != "next" {
		t.Fatal("new session was removed by old disconnect", err)
	}
	service.Disconnect(ctx, TrustedIdentity{EnterpriseID: enterprise, ConnectorID: connector}, 1)
	if _, err = store.Queries.GetConnectorSession(ctx, db.GetConnectorSessionParams{ConnectorID: connector, ConnectionEpoch: 2}); err != nil {
		t.Fatal("late repeated disconnect removed the current session", err)
	}
}
