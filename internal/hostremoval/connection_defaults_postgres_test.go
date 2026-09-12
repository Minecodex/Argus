package hostremoval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestRemovalProvenanceAndProjectionPostgres(t *testing.T) {
	if os.Getenv("ARGUS_REMOVAL_PROVENANCE_INTEGRATION") != "1" {
		t.Skip("set ARGUS_REMOVAL_PROVENANCE_INTEGRATION=1 to run the disposable PostgreSQL test")
	}
	ctx := context.Background()
	container := "argus-removal-provenance-pg-" + uuid.NewString()[:8]
	dockerTest(t, "run", "-d", "--name", container, "-e", "POSTGRES_PASSWORD=removal-provenance-test-only", "-p", "127.0.0.1::5432", "postgres:18.6-alpine")
	t.Cleanup(func() { dockerTest(t, "rm", "-f", container) })
	address := strings.TrimSpace(dockerTest(t, "port", container, "5432/tcp"))
	databaseURL := "postgres://postgres:removal-provenance-test-only@" + address + "/postgres?sslmode=disable"
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

	enterpriseID, scopeID := uuid.New(), uuid.New()
	managedHostID, rootHostID := uuid.New(), uuid.New()
	currentHostConnectorID, replacementHostConnectorID, oldHostConnectorID := uuid.New(), uuid.New(), uuid.New()
	currentBastionConnectorID, replacementBastionConnectorID, oldBastionConnectorID := uuid.New(), uuid.New(), uuid.New()
	releaseID, connectionTestID, actorID := uuid.New(), uuid.New(), uuid.New()
	hostOperationID, bastionOperationID := uuid.New(), uuid.New()
	hostRemovalID, bastionRemovalID := uuid.New(), uuid.New()

	mustSQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Removal provenance',$2,'UTC')", enterpriseID, "removal-provenance-"+enterpriseID.String())
	mustSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status,removal_generation) VALUES($1,$2,'Bastion','development',decode(repeat('00',32),'hex'),'command','active',1)", scopeID, enterpriseID)
	mustSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status,removal_generation) VALUES($1,$2,'Managed','192.0.2.10',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'managed_host','direct',NULL,'active',1),($3,$2,'Bastion','192.0.2.20',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','direct',$4,'active',1)", managedHostID, enterpriseID, rootHostID, scopeID)
	for _, connector := range []struct {
		id, hostID uuid.UUID
		role       string
	}{{currentHostConnectorID, managedHostID, "host"}, {replacementHostConnectorID, managedHostID, "host"}, {oldHostConnectorID, managedHostID, "host"},
		{currentBastionConnectorID, rootHostID, "bastion"}, {replacementBastionConnectorID, rootHostID, "bastion"}, {oldBastionConnectorID, rootHostID, "bastion"}} {
		mustSQL(t, store, "INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,$3,$6,$4,$5,$6,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 day','online',1)", connector.id, enterpriseID, connector.role, connector.hostID, nullableScope(connector.role, scopeID), connector.id.String())
	}
	mustSQL(t, store, "UPDATE hosts SET connector_id=$2 WHERE id=$1", managedHostID, currentHostConnectorID)
	mustSQL(t, store, "UPDATE hosts SET connector_id=$2,bastion_scope_id=$3 WHERE id=$1", rootHostID, currentBastionConnectorID, scopeID)
	mustSQL(t, store, "UPDATE bastion_scopes SET connector_host_id=$2,active_connector_id=$3 WHERE id=$1", scopeID, rootHostID, currentBastionConnectorID)
	mustSQL(t, store, "INSERT INTO connector_release_versions(id,version,manifest,manifest_hash) VALUES($1,'provenance-test','{}',decode(repeat('00',32),'hex'))", releaseID)
	mustSQL(t, store, "INSERT INTO connection_tests(id,enterprise_id,target_type,path,request_plan,request_hash,status,result,expires_at,created_by) VALUES($1,$2,'host','direct','{}',decode(repeat('00',32),'hex'),'succeeded','{}',now()+interval '1 hour',$3)", connectionTestID, enterpriseID, actorID)

	actionIDs := make([]uuid.UUID, 6)
	for index := range actionIDs {
		actionIDs[index] = uuid.New()
		mustSQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,impact_hash,expires_at) VALUES($1,$4,$2,$3,1,'host.create','test','test','write','{}','failed','host',decode(repeat('00',32),'hex'),now()+interval '1 hour')", actionIDs[index], enterpriseID, actorID, actionIDs[index].String())
	}
	latestFailedHostOperationID := uuid.New()
	mustSQL(t, store, "INSERT INTO host_onboarding_operations(id,enterprise_id,host_id,connector_id,pending_action_id,release_version_id,connection_test_id,install_method,ssh_path,target_platform,control_path,plan,plan_hash,status,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,'ssh','direct_executor','linux_amd64','direct','{}',decode(repeat('00',32),'hex'),'failed',now()+interval '1 hour',now()-interval '2 minutes'),($8,$2,$3,$9,$10,$6,NULL,'manual','none','linux_amd64','direct','{}',decode(repeat('00',32),'hex'),'failed',now()+interval '1 hour',now()-interval '1 minute')", hostOperationID, enterpriseID, managedHostID, currentHostConnectorID, actionIDs[0], releaseID, connectionTestID, latestFailedHostOperationID, replacementHostConnectorID, actionIDs[1])
	mustSQL(t, store, "INSERT INTO connector_install_operations(id,enterprise_id,connector_id,bastion_scope_id,host_id,pending_action_id,release_version_id,connection_test_id,install_mode,plan,plan_hash,status,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'direct_install','{}',decode(repeat('00',32),'hex'),'failed',now()+interval '1 hour',now()-interval '2 minutes'),($9,$2,$10,$4,$5,$11,$7,$8,'direct_install','{}',decode(repeat('00',32),'hex'),'failed',now()+interval '1 hour',now()-interval '1 minute')", bastionOperationID, enterpriseID, currentBastionConnectorID, scopeID, rootHostID, actionIDs[2], releaseID, connectionTestID, uuid.New(), replacementBastionConnectorID, actionIDs[3])
	mustSQL(t, store, "INSERT INTO host_removal_operations(id,enterprise_id,pending_action_id,target_type,host_id,connector_id,removal_mode,delivery_method,ssh_path,target_platform,control_path,resource_version,connector_version,connection_epoch,removal_generation,trust_bundle_epoch,plan,plan_hash,status,stage,local_cleanup,expires_at) VALUES($1,$2,$3,'managed_host',$4,$5,'uninstall','manual','none','linux_amd64','direct',1,1,1,1,1,'{}',decode(repeat('00',32),'hex'),'succeeded','completed','verified',now()+interval '1 hour')", hostRemovalID, enterpriseID, actionIDs[4], managedHostID, oldHostConnectorID)
	mustSQL(t, store, "INSERT INTO host_removal_operations(id,enterprise_id,pending_action_id,target_type,host_id,bastion_scope_id,connector_id,removal_mode,delivery_method,ssh_path,target_platform,control_path,resource_version,connector_version,connection_epoch,removal_generation,trust_bundle_epoch,plan,plan_hash,status,stage,local_cleanup,expires_at) VALUES($1,$2,$3,'bastion_scope',$4,$5,$6,'uninstall','manual','none','linux_amd64','direct',1,1,1,1,1,'{}',decode(repeat('00',32),'hex'),'succeeded','completed','verified',now()+interval '1 hour')", bastionRemovalID, enterpriseID, actionIDs[5], rootHostID, scopeID, oldBastionConnectorID)

	hostOrigin, err := store.Queries.GetLatestHostOnboardingOperationByConnector(ctx, db.GetLatestHostOnboardingOperationByConnectorParams{HostID: managedHostID, ConnectorID: currentHostConnectorID, EnterpriseID: enterpriseID})
	if err != nil || hostOrigin.ID != hostOperationID {
		t.Fatalf("current Host provenance = %s %v", hostOrigin.ID, err)
	}
	bastionOrigin, err := store.Queries.GetLatestConnectorInstallOperation(ctx, db.GetLatestConnectorInstallOperationParams{ConnectorID: currentBastionConnectorID, EnterpriseID: enterpriseID})
	if err != nil || bastionOrigin.ID != bastionOperationID {
		t.Fatalf("current Bastion provenance = %s %v", bastionOrigin.ID, err)
	}
	assertHostOnboardingProjection(t, store, enterpriseID, managedHostID, hostOperationID, "ssh", "direct_executor")
	mustSQL(t, store, "UPDATE hosts SET connector_id=NULL WHERE id=$1", managedHostID)
	assertHostOnboardingProjection(t, store, enterpriseID, managedHostID, latestFailedHostOperationID, "manual", "none")
	mustSQL(t, store, "UPDATE hosts SET connector_id=$2 WHERE id=$1", managedHostID, currentHostConnectorID)

	assertRemovalProjection(t, store, enterpriseID, managedHostID, scopeID, uuid.Nil, uuid.Nil)
	mustSQL(t, store, "UPDATE hosts SET status='uninstalled',connector_id=$2 WHERE id=$1", managedHostID, oldHostConnectorID)
	mustSQL(t, store, "UPDATE bastion_scopes SET status='uninstalled',active_connector_id=NULL WHERE id=$1", scopeID)
	mustSQL(t, store, "UPDATE hosts SET status='uninstalled',connector_id=$2 WHERE id=$1", rootHostID, oldBastionConnectorID)
	assertRemovalProjection(t, store, enterpriseID, managedHostID, scopeID, hostRemovalID, bastionRemovalID)
	mustSQL(t, store, "UPDATE bastion_scopes SET status='pending' WHERE id=$1", scopeID)
	assertRemovalProjection(t, store, enterpriseID, managedHostID, scopeID, hostRemovalID, uuid.Nil)
	assertUnregisteredBastionCancellation(t, store, enterpriseID, releaseID, connectionTestID, actorID)
}

func assertUnregisteredBastionCancellation(t *testing.T, store *postgres.Store, enterpriseID, releaseID, connectionTestID, actorID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	scopeID, hostID, connectorID, operationID, actionID, tokenID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	secretID, secretVersionID, credentialID, tunnelID, leaseID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,'Pending bastion','development',decode(repeat('00',32),'hex'),'direct_install_tunnel','pending')", scopeID, enterpriseID)
	mustSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status) VALUES($1,$2,'Pending bastion','192.0.2.30',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','executor_tunnel',$3,'active')", hostID, enterpriseID, scopeID)
	mustSQL(t, store, "UPDATE bastion_scopes SET connector_host_id=$2 WHERE id=$1", scopeID, hostID)
	mustSQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,impact_hash,expires_at) VALUES($1,$4,$2,$3,1,'bastion_scope.create','test','test','write','{}','executing','bastion_scope',decode(repeat('00',32),'hex'),now()+interval '1 hour')", actionID, enterpriseID, actorID, actionID.String())
	mustSQL(t, store, "INSERT INTO connector_install_operations(id,enterprise_id,connector_id,bastion_scope_id,host_id,pending_action_id,release_version_id,connection_test_id,install_mode,plan,plan_hash,status,lease_owner,lease_expires_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'direct_install_tunnel','{}',decode(repeat('00',32),'hex'),'running','executor-a',now()+interval '1 minute',now()+interval '1 hour')", operationID, enterpriseID, connectorID, scopeID, hostID, actionID, releaseID, connectionTestID)
	mustSQL(t, store, "INSERT INTO connector_install_operation_secrets(operation_id,enterprise_id,key_version,nonce,ciphertext,expires_at) VALUES($1,$2,1,decode(repeat('00',12),'hex'),decode('01','hex'),now()+interval '1 hour')", operationID, enterpriseID)
	mustSQL(t, store, "INSERT INTO connector_enrollment_tokens(id,preallocated_connector_id,enterprise_id,role,purpose,bastion_scope_id,preallocated_host_id,token_hash,policy,status,expires_at,created_by,release_version_id) VALUES($1,$2,$3,'bastion','initial_registration',$4,$5,decode(repeat('01',32),'hex'),'{}','active',now()+interval '1 hour',$6,$7)", tokenID, connectorID, enterpriseID, scopeID, hostID, actorID, releaseID)
	mustSQL(t, store, "INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'shared bastion credential','ssh_password',$3)", secretID, enterpriseID, actorID)
	mustSQL(t, store, "INSERT INTO secret_versions(id,secret_id,enterprise_id,version,key_id,key_version,wrapped_dek,wrap_nonce,nonce,ciphertext,value_hash) VALUES($1,$2,$3,1,'test',1,decode('01','hex'),decode(repeat('00',12),'hex'),decode(repeat('00',12),'hex'),decode('01','hex'),decode(repeat('00',32),'hex'))", secretVersionID, secretID, enterpriseID)
	mustSQL(t, store, "INSERT INTO credentials(id,enterprise_id,name,protocol,username,secret_id) VALUES($1,$2,'shared bastion ssh','ssh','root',$3)", credentialID, enterpriseID, secretID)
	mustSQL(t, store, "INSERT INTO connector_control_tunnels(id,enterprise_id,connector_id,bastion_scope_id,host_id,credential_id,credential_version,target_address,target_port,target_username,pinned_host_key,enroll_forward_target,gateway_forward_target,status,epoch,fence,lease_owner,lease_expires_at) VALUES($1,$2,$3,$4,$5,$6,1,'192.0.2.30',22,'root','SHA256:test','enroll:443','gateway:9443','established',4,7,'executor-a',now()+interval '1 minute')", tunnelID, enterpriseID, connectorID, scopeID, hostID, credentialID)
	mustSQL(t, store, "INSERT INTO credential_leases(id,enterprise_id,credential_id,secret_version_id,operation_ref,target_resource_type,target_resource_id,recipient_type,recipient_id,protocol,status,expires_at) VALUES($1,$2,$3,$4,$5,'host',$6,'direct_executor','executor-a','ssh','active',now()+interval '1 minute')", leaseID, enterpriseID, credentialID, secretVersionID, "connector_control_tunnel:"+tunnelID.String(), hostID)
	if err := store.InTx(ctx, func(q *db.Queries) error {
		if err := hostonboarding.CancelBastion(ctx, q, enterpriseID, scopeID); err != nil {
			return err
		}
		if _, err := q.DeleteUnregisteredBastionRootHost(ctx, db.DeleteUnregisteredBastionRootHostParams{ID: hostID, EnterpriseID: enterpriseID, BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true}}); err != nil {
			return err
		}
		_, err := q.DeleteBastionScope(ctx, db.DeleteBastionScopeParams{ID: scopeID, EnterpriseID: enterpriseID, ResourceVersion: 1})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	operation, err := store.Queries.GetConnectorInstallOperation(ctx, db.GetConnectorInstallOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil || operation.Status != "cancelled" || operation.ErrorCode.String != "CONNECTOR_INSTALL_CANCELLED_BY_DELETE" {
		t.Fatalf("Bastion operation not cancelled: %+v %v", operation, err)
	}
	var tokenStatus, leaseStatus, tunnelStatus, credentialStatus string
	if err = store.Pool.QueryRow(ctx, "SELECT token.status,lease.status,tunnel.status,credential.status FROM connector_enrollment_tokens token,credential_leases lease,connector_control_tunnels tunnel,credentials credential WHERE token.bastion_scope_id=$1 AND lease.id=$2 AND tunnel.id=$3 AND credential.id=$4", scopeID, leaseID, tunnelID, credentialID).Scan(&tokenStatus, &leaseStatus, &tunnelStatus, &credentialStatus); err != nil {
		t.Fatal(err)
	}
	if tokenStatus != "revoked" || leaseStatus != "revoked" || tunnelStatus != "removed" || credentialStatus != "active" {
		t.Fatalf("Bastion cleanup = token %s lease %s tunnel %s credential %s", tokenStatus, leaseStatus, tunnelStatus, credentialStatus)
	}
	var secretCount int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM connector_install_operation_secrets WHERE operation_id=$1", operationID).Scan(&secretCount); err != nil || secretCount != 0 {
		t.Fatalf("temporary operation secret count = %d %v", secretCount, err)
	}
	if _, err = store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{ID: scopeID, EnterpriseID: enterpriseID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted Bastion remained visible: %v", err)
	}
	if _, err = store.Queries.ConsumeEnrollmentToken(ctx, db.ConsumeEnrollmentTokenParams{ID: tokenID, ConsumedDeviceHash: make([]byte, 32), RegisteredConnectorID: uuid.NullUUID{UUID: connectorID, Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("revoked token was consumed after delete: %v", err)
	}
}

func assertHostOnboardingProjection(t *testing.T, store *postgres.Store, enterpriseID, hostID, wantOperationID uuid.UUID, wantMethod, wantPath string) {
	t.Helper()
	facts, err := store.Queries.ListHostOnboardingFacts(context.Background(), db.ListHostOnboardingFactsParams{EnterpriseID: enterpriseID, Column2: []uuid.UUID{hostID}})
	if err != nil || len(facts) != 1 {
		t.Fatalf("Host onboarding projection unavailable: %+v %v", facts, err)
	}
	if facts[0].OperationID != wantOperationID || facts[0].InstallMethod != wantMethod || facts[0].SshPath != wantPath {
		t.Fatalf("Host onboarding projection = operation %s method %s path %s, want %s/%s/%s", facts[0].OperationID,
			facts[0].InstallMethod, facts[0].SshPath, wantOperationID, wantMethod, wantPath)
	}
}

func assertRemovalProjection(t *testing.T, store *postgres.Store, enterpriseID, hostID, scopeID, wantHost, wantScope uuid.UUID) {
	t.Helper()
	facts, err := store.Queries.ListHostOnboardingFacts(context.Background(), db.ListHostOnboardingFactsParams{EnterpriseID: enterpriseID, Column2: []uuid.UUID{hostID}})
	if err != nil || len(facts) != 1 || facts[0].RemovalOperationID != wantHost {
		t.Fatalf("Host removal projection = %+v %v, want %s", facts, err, wantHost)
	}
	scope, err := store.Queries.GetBastionScope(context.Background(), db.GetBastionScopeParams{ID: scopeID, EnterpriseID: enterpriseID})
	if err != nil || scope.RemovalOperationID != wantScope {
		t.Fatalf("Bastion removal projection = %s %v, want %s", scope.RemovalOperationID, err, wantScope)
	}
	scopes, err := store.Queries.ListBastionScopes(context.Background(), enterpriseID)
	if err != nil || len(scopes) != 1 || scopes[0].RemovalOperationID != wantScope {
		t.Fatalf("Bastion list removal projection = %+v %v, want %s", scopes, err, wantScope)
	}
}

func nullableScope(role string, scopeID uuid.UUID) any {
	if role == "bastion" {
		return scopeID
	}
	return nil
}
