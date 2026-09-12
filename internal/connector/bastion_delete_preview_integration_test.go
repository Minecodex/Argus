package connector

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	actiondomain "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestBastionDeletePreviewRiskPostgres(t *testing.T) {
	if os.Getenv("ARGUS_BASTION_DELETE_PREVIEW_INTEGRATION") != "1" {
		t.Skip("set ARGUS_BASTION_DELETE_PREVIEW_INTEGRATION=1 to run the disposable PostgreSQL test")
	}
	ctx := context.Background()
	container := "argus-bastion-delete-preview-" + uuid.NewString()[:8]
	bastionPreviewDocker(t, "run", "-d", "--name", container, "-e", "POSTGRES_PASSWORD=bastion-preview-test", "-p", "127.0.0.1::5432", "postgres:18.6-alpine")
	t.Cleanup(func() { bastionPreviewDocker(t, "rm", "-f", container) })
	address := strings.TrimSpace(bastionPreviewDocker(t, "port", container, "5432/tcp"))
	databaseURL := "postgres://postgres:bastion-preview-test@" + address + "/postgres?sslmode=disable"
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
	root, _ := filepath.Abs("../..")
	if err = postgres.RunMigrations(ctx, databaseURL, filepath.Join(root, "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	enterpriseID, scopeID, hostID, actorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bastionPreviewSQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Bastion preview',$2,'UTC')", enterpriseID, "bastion-preview-"+enterpriseID.String())
	bastionPreviewSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,'Pending bastion','development',decode(repeat('00',32),'hex'),'direct_install','pending')", scopeID, enterpriseID)
	bastionPreviewSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status) VALUES($1,$2,'Pending bastion','192.0.2.40',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','direct',$3,'active')", hostID, enterpriseID, scopeID)
	bastionPreviewSQL(t, store, "UPDATE bastion_scopes SET connector_host_id=$2 WHERE id=$1", scopeID, hostID)
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}, Key: bytes.Repeat([]byte{4}, 32)}
	service := BastionService{Store: store, Actions: actions}
	subject := resource.Subject{ActorID: actorID.String(), AuthorizationVersion: 1, AuthorizedResourceIDs: []uuid.UUID{hostID}}
	preview, err := service.PreviewLifecycle(ctx, subject, enterpriseID, scopeID, 1, "delete", "delete-pending-bastion")
	if err != nil || preview.Risk != "write" {
		t.Fatalf("pending Bastion delete preview = risk %q error %v", preview.Risk, err)
	}
	bastionPreviewSQL(t, store, "UPDATE bastion_scopes SET status='uninstalled' WHERE id=$1", scopeID)
	bastionPreviewSQL(t, store, "UPDATE hosts SET status='uninstalled' WHERE id=$1", hostID)
	preview, err = service.PreviewLifecycle(ctx, subject, enterpriseID, scopeID, 1, "delete", "delete-uninstalled-bastion")
	if err != nil || preview.Risk != "dangerous" {
		t.Fatalf("uninstalled Bastion delete preview = risk %q error %v", preview.Risk, err)
	}
	assertCancelledActionReconciliation(t, store, enterpriseID, actorID)
}

func assertCancelledActionReconciliation(t *testing.T, store *postgres.Store, enterpriseID, actorID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	releaseID, connectionTestID := uuid.New(), uuid.New()
	hostID, hostOperationID, failedOperationID := uuid.New(), uuid.New(), uuid.New()
	scopeID, rootHostID, bastionOperationID := uuid.New(), uuid.New(), uuid.New()
	hostActionID, bastionActionID, failedActionID, hostExecutionID, bastionExecutionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bastionPreviewSQL(t, store, "INSERT INTO connector_release_versions(id,version,manifest,manifest_hash) VALUES($1,$2,'{}',decode(repeat('00',32),'hex'))", releaseID, "reconcile-"+releaseID.String())
	bastionPreviewSQL(t, store, "INSERT INTO connection_tests(id,enterprise_id,target_type,path,request_plan,request_hash,status,result,expires_at,created_by) VALUES($1,$2,'host','direct','{}',decode(repeat('00',32),'hex'),'succeeded','{}',now()+interval '1 hour',$3)", connectionTestID, enterpriseID, actorID)
	bastionPreviewSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,status) VALUES($1,$2,'Deleted Host','',0,'linux','amd64','development',decode(repeat('00',32),'hex'),'managed_host','direct','deleted')", hostID, enterpriseID)
	bastionPreviewSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,'Deleted Scope','development',decode(repeat('00',32),'hex'),'direct_install','deleted')", scopeID, enterpriseID)
	bastionPreviewSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status) VALUES($1,$2,'Deleted Root','',0,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','direct',$3,'deleted')", rootHostID, enterpriseID, scopeID)
	for _, item := range []struct{ actionID, executionID uuid.UUID }{{hostActionID, hostExecutionID}, {bastionActionID, bastionExecutionID}} {
		bastionPreviewSQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,result_resource_type,result_resource_id,result_resource_version,result_summary,impact_hash,expires_at) VALUES($1,$5,$2,$3,1,'host.delete','Delete','Delete','write','{}','executing','host','host',$4,7,'preserved result',decode(repeat('00',32),'hex'),now()+interval '1 hour')", item.actionID, enterpriseID, actorID, hostID, item.actionID.String())
		bastionPreviewSQL(t, store, "INSERT INTO executions(id,execution_ref,pending_action_id,enterprise_id,status,idempotency_key) VALUES($1,$4,$2,$3,'result_unknown',$4)", item.executionID, item.actionID, enterpriseID, item.executionID.String())
	}
	bastionPreviewSQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,impact_hash,expires_at) VALUES($1,$4,$2,$3,1,'host.create','Create','Create','write','{}','failed','host',decode(repeat('00',32),'hex'),now()+interval '1 hour')", failedActionID, enterpriseID, actorID, failedActionID.String())
	bastionPreviewSQL(t, store, "INSERT INTO host_onboarding_operations(id,enterprise_id,host_id,connector_id,pending_action_id,release_version_id,install_method,ssh_path,target_platform,control_path,plan,plan_hash,status,error_code,expires_at) VALUES($1,$2,$3,$4,$5,$6,'manual','none','linux_amd64','direct','{}',decode(repeat('00',32),'hex'),'cancelled','HOST_ONBOARDING_CANCELLED_BY_DELETE',now()+interval '1 hour'),($7,$2,$3,$8,$9,$6,'manual','none','linux_amd64','direct','{}',decode(repeat('00',32),'hex'),'failed','ORIGINAL_FAILURE',now()+interval '1 hour')", hostOperationID, enterpriseID, hostID, uuid.New(), hostActionID, releaseID, failedOperationID, uuid.New(), failedActionID)
	bastionPreviewSQL(t, store, "INSERT INTO connector_install_operations(id,enterprise_id,connector_id,bastion_scope_id,host_id,pending_action_id,release_version_id,connection_test_id,install_mode,plan,plan_hash,status,error_code,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'direct_install','{}',decode(repeat('00',32),'hex'),'cancelled','CONNECTOR_INSTALL_CANCELLED_BY_DELETE',now()+interval '1 hour')", bastionOperationID, enterpriseID, uuid.New(), scopeID, rootHostID, bastionActionID, releaseID, connectionTestID)
	bastionPreviewSQL(t, store, "UPDATE executions SET host_onboarding_operation_id=$2 WHERE id=$1", hostExecutionID, hostOperationID)
	bastionPreviewSQL(t, store, "UPDATE executions SET connector_install_operation_id=$2 WHERE id=$1", bastionExecutionID, bastionOperationID)
	reconcileCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- (actiondomain.Reconciler{Executor: actiondomain.Executor{Store: store}, Poll: 10 * time.Millisecond}).Run(reconcileCtx)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM executions WHERE id IN ($1,$2) AND status='cancelled'", hostExecutionID, bastionExecutionID).Scan(&count); err == nil && count == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ actionID, executionID uuid.UUID }{{hostActionID, hostExecutionID}, {bastionActionID, bastionExecutionID}} {
		var actionStatus, executionStatus, resourceType, summary string
		var resourceID uuid.UUID
		var resourceVersion int64
		if err := store.Pool.QueryRow(ctx, "SELECT action.status,execution.status,action.result_resource_type,action.result_resource_id,action.result_resource_version,action.result_summary FROM pending_actions action JOIN executions execution ON execution.pending_action_id=action.id WHERE action.id=$1", item.actionID).Scan(&actionStatus, &executionStatus, &resourceType, &resourceID, &resourceVersion, &summary); err != nil {
			t.Fatal(err)
		}
		if actionStatus != "cancelled" || executionStatus != "cancelled" || resourceType != "host" || resourceID != hostID || resourceVersion != 7 || summary == "" {
			t.Fatalf("reconciled action/execution = %s/%s resource %s/%s/%d summary %q", actionStatus, executionStatus, resourceType, resourceID, resourceVersion, summary)
		}
	}
	failed, err := store.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: failedOperationID, EnterpriseID: enterpriseID})
	if err != nil || failed.Status != "failed" || failed.ErrorCode.String != "ORIGINAL_FAILURE" {
		t.Fatalf("historical failed operation changed: %+v %v", failed, err)
	}
}

func bastionPreviewSQL(t *testing.T, store *postgres.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func bastionPreviewDocker(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args, err, output)
	}
	return string(output)
}
