package workspace

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestWorkspaceDeletionFencesNewWritersAndPreservesArchive(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated PostgreSQL database")
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enterprise, department, user, model, convo, workspace := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'P5 workspace',$2,'UTC')", enterprise, "p5-"+enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'P5 user')", user, enterprise, department, "p5-"+user.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, enterprise)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'P5',$4)", convo, enterprise, user, model)
	oldRun := uuid.New()
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", oldRun, convo, enterprise, user, model)
	exec("INSERT INTO workspace_quotas(enterprise_id,limit_bytes,reserved_bytes) VALUES($1,21474836480,2147483648)", enterprise)
	exec("INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version) VALUES($1,$2,$3,$4,'test-workspace','ready',2147483648,'test')", workspace, enterprise, convo, "workspace-"+workspace.String())
	principal := (toolruntime.Principal{EnterpriseID: enterprise, UserID: user, ConversationID: convo, Permissions: []string{"workspace.use"}})
	actor, err := store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: user, EnterpriseID: enterprise})
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthorizationVersion = actor.AuthorizationVersion
	exec("UPDATE conversations SET status='archived' WHERE id=$1", convo)
	value, err := (Service{Store: store}).Get(ctx, principal)
	if err != nil || value.ID != workspace {
		t.Fatalf("archive discarded Workspace: %v", err)
	}
	until := pgtype.Interval{Microseconds: 30_000_000, Valid: true}
	first, err := store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "writer-a", Valid: true}, Column4: until})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "writer-b", Valid: true}, Column4: until}); err == nil {
		t.Fatal("two writers obtained the same live Workspace")
	}
	// Simulate delayed cleanup from a former owner after a different owner has
	// already acquired the same Workspace. It must issue no Kubernetes mutation.
	exec("UPDATE workspaces SET lease_until=now()-interval '1 second' WHERE id=$1", workspace)
	if _, err := store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "stale-snapshot", Valid: true}, Column4: until, ExpectedFence: first.FenceToken - 1}); err == nil {
		t.Fatal("claim accepted an obsolete clean/abandoned snapshot")
	}
	next, err := store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "writer-b", Valid: true}, Column4: until, ExpectedFence: first.FenceToken})
	if err != nil {
		t.Fatal(err)
	}
	guardClient := fake.NewSimpleClientset()
	guard := Service{Store: store, Kubernetes: Kubernetes{Client: guardClient}}
	stale := guard.cleanupAccess(ctx, first, "writer-a")
	if err := stale.Close(); err == nil {
		t.Fatal("old owner cleanup did not report its lost lease")
	}
	if len(guardClient.Actions()) != 0 {
		t.Fatal("old owner mutated Kubernetes after takeover")
	}
	current, err := store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: workspace, EnterpriseID: enterprise})
	if err != nil || current.LeaseOwner != next.LeaseOwner || current.FenceToken != next.FenceToken {
		t.Fatalf("old owner overwrote the new lease: %v", err)
	}
	service := Service{Store: store, Idempotency: postgres.Idempotency{Key: []byte(strings.Repeat("k", 32))}}
	deleted, err := service.Delete(ctx, principal, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != "deleting" {
		t.Fatal("deletion intent missing")
	}
	if _, err := store.Queries.ClaimWorkspaceDeletion(ctx, db.ClaimWorkspaceDeletionParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "cleanup", Valid: true}, Column4: until}); err == nil {
		t.Fatal("cleanup bypassed a live writer lease")
	}
	exec("UPDATE workspaces SET lease_until=$2 WHERE id=$1", workspace, time.Now().Add(-time.Second))
	if _, err := store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "new-writer", Valid: true}, Column4: until}); err == nil {
		t.Fatal("new writer attached to deleting Workspace")
	}
	cleanup, err := store.Queries.ClaimWorkspaceDeletion(ctx, db.ClaimWorkspaceDeletionParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "cleanup", Valid: true}, Column4: until})
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.FenceToken <= first.FenceToken {
		t.Fatal("cleanup fence did not advance")
	}
	if count, err := store.Queries.SetWorkspaceAttachment(ctx, db.SetWorkspaceAttachmentParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: pgtype.Text{String: "writer-a", Valid: true}, FenceToken: first.FenceToken}); err != nil || count != 0 {
		t.Fatal("old writer retained attachment authority")
	}
	// Simulate completed cleanup before a formerly authorized operation gets
	// scheduled again. Neither an old Run nor a late file request may recreate.
	if count, err := store.Queries.FinishWorkspaceDeletion(ctx, db.FinishWorkspaceDeletionParams{ID: workspace, EnterpriseID: enterprise, LeaseOwner: cleanup.LeaseOwner, FenceToken: cleanup.FenceToken}); err != nil || count != 1 {
		t.Fatalf("cleanup did not commit its deletion tombstone: count=%d error=%v", count, err)
	}
	classMode := storagev1.VolumeBindingWaitForFirstConsumer
	client := fake.NewSimpleClientset(&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "argus-workspace"}, Provisioner: "rawfile.csi.openebs.io", Parameters: map[string]string{"thinProvision": "false", "csi.storage.k8s.io/fstype": "ext4"}, MountOptions: []string{"nodiscard"}, VolumeBindingMode: &classMode})
	cfg := config.Workspace{Enabled: true, Namespace: "test-workspace", StorageClass: "argus-workspace", IOImage: "fixed-io", DefaultBytes: 2 << 30, EnterpriseBytes: 20 << 30}
	service.Config, service.Kubernetes = cfg, Kubernetes{Client: client, Config: cfg}
	for _, scope := range []accessScope{{RunID: oldRun}, {WorkspaceID: workspace}} {
		if _, err := service.ensure(ctx, principal, scope); err == nil {
			t.Fatal("late operation recreated a deleted Workspace")
		}
	}
	var count int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM workspaces WHERE conversation_id=$1", convo).Scan(&count); err != nil || count != 1 {
		t.Fatalf("late operation allocated a replacement workspace: count=%d error=%v", count, err)
	}

	// Force permanent deletion between the initial authorization and the
	// conversation lock. A runless user request must recheck the locked row.
	checked := make(chan struct{}, 1)
	client.PrependReactor("get", "storageclasses", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		select {
		case checked <- struct{}{}:
		default:
		}
		return false, nil, nil
	})
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM conversations WHERE id=$1 FOR UPDATE", convo); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := service.ensure(ctx, principal, accessScope{}); finished <- err }()
	select {
	case <-checked:
	case <-time.After(5 * time.Second):
		t.Fatal("file request did not reach its post-authorization check")
	}
	if _, err := tx.Exec(ctx, "UPDATE conversations SET status='deleted' WHERE id=$1", convo); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("a stale authorization recreated files after permanent conversation deletion")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("file request did not finish after the conversation lock was released")
	}
	// Restore only this fixture's conversation so the public deletion service
	// is still exercised below, including its access tombstone.
	exec("UPDATE conversations SET status='archived' WHERE id=$1", convo)
	if _, err := (conversation.Service{Store: store, Idempotency: service.Idempotency}).Delete(ctx, enterprise, user, convo, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := (conversation.Service{Store: store}).Get(ctx, enterprise, user, convo); err == nil {
		t.Fatal("permanently deleted conversation remains accessible")
	}
}
