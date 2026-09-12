package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type retryOnboardingStub struct {
	operationID, connectorID, releaseID, connectionTestID uuid.UUID
}

func (retryOnboardingStub) PrepareHostConnectorOnboarding(context.Context, *db.Queries, uuid.UUID, uuid.UUID, HostInput, string) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (retryOnboardingStub) RevalidateHostConnectorOnboarding(context.Context, *db.Queries, uuid.UUID, json.RawMessage) error {
	return nil
}

func (stub retryOnboardingStub) CommitHostConnectorOnboarding(ctx context.Context, q *db.Queries, action db.PendingAction, host db.Host, _ json.RawMessage) (ActionCommitResult, error) {
	operation, err := q.CreateHostOnboardingOperation(ctx, db.CreateHostOnboardingOperationParams{
		ID: stub.operationID, EnterpriseID: action.EnterpriseID, HostID: host.ID, ConnectorID: stub.connectorID,
		PendingActionID: action.ID, ReleaseVersionID: stub.releaseID,
		ConnectionTestID: uuid.NullUUID{UUID: stub.connectionTestID, Valid: true}, InstallMethod: "ssh", SshPath: "direct_executor",
		TargetPlatform: "linux_amd64", ControlPath: "executor_tunnel", Plan: []byte(`{}`), PlanHash: make([]byte, 32),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		return ActionCommitResult{}, err
	}
	return ActionCommitResult{HostOnboardingOperationID: uuid.NullUUID{UUID: operation.ID, Valid: true}}, nil
}

func TestRetryRetiresOnlyPreviousHostControlTunnel(t *testing.T) {
	if os.Getenv("ARGUS_HOST_RETRY_INTEGRATION") != "1" {
		t.Skip("set ARGUS_HOST_RETRY_INTEGRATION=1 to run the disposable PostgreSQL test")
	}
	ctx := context.Background()
	container := "argus-host-retry-pg-" + uuid.NewString()[:8]
	hostRetryDocker(t, "run", "-d", "--name", container, "-e", "POSTGRES_PASSWORD=host-retry-test-only", "-p", "127.0.0.1::5432", "postgres:18.6-alpine")
	t.Cleanup(func() { hostRetryDocker(t, "rm", "-f", container) })
	address := strings.TrimSpace(hostRetryDocker(t, "port", container, "5432/tcp"))
	databaseURL := "postgres://postgres:host-retry-test-only@" + address + "/postgres?sslmode=disable"
	var store *postgres.Store
	var err error
	for range 40 {
		store, err = postgres.Open(ctx, databaseURL)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err = postgres.RunMigrations(ctx, databaseURL, filepath.Join(root, "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}

	enterpriseID := uuid.New()
	hostID := uuid.New()
	otherHostID := uuid.New()
	previousOperationID := uuid.New()
	targetTunnelID := uuid.New()
	sameHostTunnelID := uuid.New()
	otherHostTunnelID := uuid.New()
	targetConnectorID := uuid.New()
	sameHostConnectorID := uuid.New()
	otherHostConnectorID := uuid.New()
	credentialID := uuid.New()
	secretID := uuid.New()
	secretVersionID := uuid.New()
	releaseID := uuid.New()
	connectionTestID := uuid.New()
	previousActionID := uuid.New()
	retryActionID := uuid.New()
	retryOperationID := uuid.New()
	retryConnectorID := uuid.New()
	actorID := uuid.New()

	hostRetrySQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Host retry test',$2,'UTC')", enterpriseID, "host-retry-"+enterpriseID.String())
	for _, item := range []struct {
		id   uuid.UUID
		name string
	}{{hostID, "retry target"}, {otherHostID, "unrelated host"}} {
		hostRetrySQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,status) VALUES($1,$2,$3,'192.0.2.10',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'managed_host','executor_tunnel','active')", item.id, enterpriseID, item.name)
	}
	hostRetrySQL(t, store, "INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'retry credential','ssh_password',$3)", secretID, enterpriseID, actorID)
	hostRetrySQL(t, store, "INSERT INTO secret_versions(id,secret_id,enterprise_id,version,key_id,key_version,wrapped_dek,wrap_nonce,nonce,ciphertext,value_hash) VALUES($1,$2,$3,1,'test',1,decode('01','hex'),decode(repeat('00',12),'hex'),decode(repeat('00',12),'hex'),decode('01','hex'),decode(repeat('00',32),'hex'))", secretVersionID, secretID, enterpriseID)
	hostRetrySQL(t, store, "INSERT INTO credentials(id,enterprise_id,name,protocol,username,secret_id) VALUES($1,$2,'retry ssh','ssh','root',$3)", credentialID, enterpriseID, secretID)
	hostRetrySQL(t, store, "INSERT INTO connector_release_versions(id,version,manifest,manifest_hash) VALUES($1,'test','{}',decode(repeat('00',32),'hex'))", releaseID)
	hostRetrySQL(t, store, "INSERT INTO connection_tests(id,enterprise_id,target_type,resource_id,path,credential_id,credential_version,request_plan,request_hash,status,result,expires_at,created_by) VALUES($1,$2,'host',$3,'direct',$4,1,'{}',decode(repeat('00',32),'hex'),'succeeded','{}',now()+interval '1 hour',$5)", connectionTestID, enterpriseID, hostID, credentialID, actorID)
	for _, action := range []struct {
		id         uuid.UUID
		actionType string
		status     string
	}{{previousActionID, "host.create", "failed"}, {retryActionID, "host.onboarding.retry", "executing"}} {
		hostRetrySQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,resource_id,impact_hash,expires_at) VALUES($1,$2,$3,$4,1,$5,'Host','Host','write','{}',$6,'host',$7,decode(repeat('00',32),'hex'),now()+interval '1 hour')", action.id, action.id.String(), enterpriseID, actorID, action.actionType, action.status, hostID)
	}
	hostRetrySQL(t, store, "INSERT INTO host_onboarding_operations(id,enterprise_id,host_id,connector_id,pending_action_id,release_version_id,connection_test_id,install_method,ssh_path,target_platform,control_path,plan,plan_hash,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,'ssh','direct_executor','linux_amd64','executor_tunnel','{}',decode(repeat('00',32),'hex'),'failed',now()+interval '1 hour')", previousOperationID, enterpriseID, hostID, targetConnectorID, previousActionID, releaseID, connectionTestID)

	for _, tunnel := range []struct {
		id, hostID, connectorID uuid.UUID
	}{{targetTunnelID, hostID, targetConnectorID}, {sameHostTunnelID, hostID, sameHostConnectorID}, {otherHostTunnelID, otherHostID, otherHostConnectorID}} {
		hostRetrySQL(t, store, "INSERT INTO connector_control_tunnels(id,enterprise_id,connector_id,host_id,credential_id,credential_version,target_address,target_port,target_username,pinned_host_key,enroll_forward_target,gateway_forward_target,status,epoch,fence,lease_owner,lease_expires_at) VALUES($1,$2,$3,$4,$5,1,'192.0.2.10',22,'root','SHA256:test','enroll:443','gateway:9443','established',4,7,'executor-a',now()+interval '1 minute')", tunnel.id, enterpriseID, tunnel.connectorID, tunnel.hostID, credentialID)
		hostRetrySQL(t, store, "INSERT INTO credential_leases(id,enterprise_id,credential_id,secret_version_id,operation_ref,target_resource_type,target_resource_id,recipient_type,recipient_id,protocol,status,expires_at) VALUES($1,$2,$3,$4,$5,'host',$6,'direct_executor','executor-a','ssh','active',now()+interval '1 minute')", uuid.New(), enterpriseID, credentialID, secretVersionID, "connector_control_tunnel:"+tunnel.id.String(), tunnel.hostID)
	}

	service := Service{HostOnboarding: retryOnboardingStub{operationID: retryOperationID, connectorID: retryConnectorID, releaseID: releaseID, connectionTestID: connectionTestID}}
	err = store.InTx(ctx, func(q *db.Queries) error {
		_, commitErr := service.commitHost(ctx, q, db.PendingAction{ID: retryActionID, EnterpriseID: enterpriseID,
			CreatorSubjectID: actorID, CreatorSubjectType: "user"}, hostActionPlan{
			Operation: "retry", HostID: hostID, RetryOf: previousOperationID,
			Input: HostInput{ExpectedVersion: 1}, PinnedKey: "SHA256:test", Arch: "amd64", OnboardingPlan: json.RawMessage(`{}`),
		})
		return commitErr
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := store.Queries.HeartbeatConnectorControlTunnel(ctx, db.HeartbeatConnectorControlTunnelParams{
		ID: targetTunnelID, EnterpriseID: enterpriseID, Fence: 7, LeaseOwner: "executor-a", BytesRelayed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("removed tunnel accepted %d stale owner heartbeat rows", rows)
	}
	assertHostRetryTunnel(t, store, targetTunnelID, "removed", 5, "", "revoked")
	assertHostRetryTunnel(t, store, sameHostTunnelID, "established", 4, "executor-a", "active")
	assertHostRetryTunnel(t, store, otherHostTunnelID, "established", 4, "executor-a", "active")
	retryOperation, err := store.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: retryOperationID, EnterpriseID: enterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	if !retryOperation.RetryOf.Valid || retryOperation.RetryOf.UUID != previousOperationID {
		t.Fatalf("retry operation did not retain previous operation identity: %+v", retryOperation.RetryOf)
	}
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	service.Store = store
	service.Actions = PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}, Key: bytes.Repeat([]byte{8}, 32)}
	deletePreview, err := service.PreviewDeleteHost(ctx, Subject{ActorID: actorID.String(), AuthorizationVersion: 1,
		AuthorizedResourceIDs: []uuid.UUID{hostID}}, enterpriseID, hostID, 2, "delete-unregistered-preview")
	if err != nil || deletePreview.Risk != "write" {
		t.Fatalf("unregistered Host delete preview = risk %q error %v", deletePreview.Risk, err)
	}
	commandConnectorID := uuid.New()
	targetCommandID, unrelatedCommandID := "cmd_cancelled_"+uuid.NewString(), "cmd_unrelated_"+uuid.NewString()
	hostRetrySQL(t, store, "INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,'host','command carrier',$3,$4,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 day','online',1)", commandConnectorID, enterpriseID, otherHostID, commandConnectorID.String())
	for _, command := range []struct{ id, operationRef string }{{targetCommandID, retryOperationID.String()}, {unrelatedCommandID, uuid.NewString()}} {
		hostRetrySQL(t, store, "INSERT INTO connector_commands(id,command_id,enterprise_id,connector_id,connection_epoch,operation_ref,command_type,payload_schema_version,payload,payload_hash,idempotency_key,status,expires_at) VALUES($1,$2,$3,$4,1,$5,'host_connector_install','argus.host_connector_install/v1','{}',decode(repeat('00',32),'hex'),$2,'running',now()+interval '1 hour')", uuid.New(), command.id, enterpriseID, commandConnectorID, command.operationRef)
	}
	rollbackMarker := errors.New("force cancellation rollback")
	err = store.InTx(ctx, func(q *db.Queries) error {
		if cancelErr := hostonboarding.CancelHost(ctx, q, enterpriseID, hostID); cancelErr != nil {
			return cancelErr
		}
		return rollbackMarker
	})
	if !errors.Is(err, rollbackMarker) {
		t.Fatalf("cancellation rollback = %v", err)
	}
	retryOperation, err = store.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: retryOperationID, EnterpriseID: enterpriseID})
	if err != nil || retryOperation.Status != "queued" {
		t.Fatalf("rolled back cancellation changed operation: %+v %v", retryOperation, err)
	}
	assertHostRetryTunnel(t, store, sameHostTunnelID, "established", 4, "executor-a", "active")
	err = store.InTx(ctx, func(q *db.Queries) error {
		_, commitErr := service.commitHost(ctx, q, db.PendingAction{EnterpriseID: enterpriseID, CreatorSubjectID: actorID, CreatorSubjectType: "user"},
			hostActionPlan{Operation: "delete", HostID: hostID, Input: HostInput{ExpectedVersion: 2}})
		return commitErr
	})
	if err != nil {
		t.Fatal(err)
	}
	retryOperation, err = store.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: retryOperationID, EnterpriseID: enterpriseID})
	if err != nil || retryOperation.Status != "cancelled" || retryOperation.ErrorCode.String != "HOST_ONBOARDING_CANCELLED_BY_DELETE" {
		t.Fatalf("active retry was not cancelled: %+v %v", retryOperation, err)
	}
	for _, next := range []string{"failed", "succeeded"} {
		_, transitionErr := store.Queries.TransitionConnectorCommand(ctx, db.TransitionConnectorCommandParams{CommandID: targetCommandID,
			ConnectorID: commandConnectorID, ConnectionEpoch: 1, ExpectedStatus: "running", Status: next})
		if !errors.Is(transitionErr, pgx.ErrNoRows) {
			t.Fatalf("cancelled command accepted %s transition: %v", next, transitionErr)
		}
	}
	targetCommand, err := store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: targetCommandID, ConnectorID: commandConnectorID, ConnectionEpoch: 1})
	if err != nil || targetCommand.Status != "expired" || targetCommand.ErrorCode.String != "HOST_ONBOARDING_CANCELLED_BY_DELETE" {
		t.Fatalf("cancelled command marker changed: %+v %v", targetCommand, err)
	}
	unrelatedCommand, err := store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: unrelatedCommandID, ConnectorID: commandConnectorID, ConnectionEpoch: 1})
	if err != nil || unrelatedCommand.Status != "running" {
		t.Fatalf("unrelated Host command changed: %+v %v", unrelatedCommand, err)
	}
	deletedHost, err := store.Queries.GetHost(ctx, db.GetHostParams{ID: hostID, EnterpriseID: enterpriseID})
	if err == nil || deletedHost.ID != uuid.Nil {
		t.Fatalf("unregistered Host remained visible: %+v %v", deletedHost, err)
	}
	assertHostRetryTunnel(t, store, sameHostTunnelID, "removed", 5, "", "revoked")
	assertHostRetryTunnel(t, store, otherHostTunnelID, "established", 4, "executor-a", "active")
	credential, err := store.Queries.GetCredential(ctx, db.GetCredentialParams{ID: credentialID, EnterpriseID: enterpriseID})
	if err != nil || credential.Status != "active" {
		t.Fatalf("shared credential changed during cancellation: %+v %v", credential, err)
	}
	hostRetrySQL(t, store, "INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,'host','registered',$3,$4,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 day','online',1)", otherHostConnectorID, enterpriseID, otherHostID, otherHostConnectorID.String())
	hostRetrySQL(t, store, "UPDATE hosts SET connector_id=$2 WHERE id=$1", otherHostID, otherHostConnectorID)
	err = store.InTx(ctx, func(q *db.Queries) error {
		_, commitErr := service.commitHost(ctx, q, db.PendingAction{EnterpriseID: enterpriseID, CreatorSubjectID: actorID, CreatorSubjectType: "user"},
			hostActionPlan{Operation: "delete", HostID: otherHostID, Input: HostInput{ExpectedVersion: 1}})
		return commitErr
	})
	if !errors.Is(err, ErrActionInvalidated) {
		t.Fatalf("registered Host direct delete = %v", err)
	}
	if _, err = store.Queries.GetHost(ctx, db.GetHostParams{ID: otherHostID, EnterpriseID: enterpriseID}); err != nil {
		t.Fatalf("registered Host was deleted: %v", err)
	}
}

func assertHostRetryTunnel(t *testing.T, store *postgres.Store, tunnelID uuid.UUID, wantStatus string, wantEpoch int64, wantOwner, wantLeaseStatus string) {
	t.Helper()
	var status, owner, leaseStatus string
	var epoch int64
	if err := store.Pool.QueryRow(context.Background(), "SELECT tunnel.status,tunnel.epoch,tunnel.lease_owner,lease.status FROM connector_control_tunnels tunnel JOIN credential_leases lease ON lease.operation_ref='connector_control_tunnel:'||tunnel.id::text WHERE tunnel.id=$1", tunnelID).Scan(&status, &epoch, &owner, &leaseStatus); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || epoch != wantEpoch || owner != wantOwner || leaseStatus != wantLeaseStatus {
		t.Fatalf("tunnel %s = status %s epoch %d owner %q lease %s", tunnelID, status, epoch, owner, leaseStatus)
	}
}

func hostRetrySQL(t *testing.T, store *postgres.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func hostRetryDocker(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args, err, output)
	}
	return string(output)
}
