package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// This audits the shared mount gate. It does not allocate a PVC or run a Pod.
func TestWorkspaceImportProvenanceSurvivesMissingMetadataAndRevocation(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, u, m, c, r, w := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Audit R12',$2,'UTC')", e, "audit-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Audit R12')", u, e, d, "audit-"+u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Audit','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Audit',$4)", c, e, u, m)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", r, c, e, u, m)
	exec("INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version) VALUES($1,$2,$3,$4,'test-workspace','ready',2147483648,'test')", w, e, c, "workspace-"+w.String())
	scope, err := presentation.Scope(ctx, store, e, u)
	if err != nil {
		t.Fatal(err)
	}
	step, call, artifact, file := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ref := "audit-r12-" + artifact.String()
	body := []byte(`{"business_value":"previously-authorized"}`)
	hash := sha256.Sum256(body)
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'tool_call','succeeded')", step, r, e)
	exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status,authorization_scope) VALUES($1,$2,$3,$4,$5,'host.list','{}',$6,'succeeded',$7)", call, call.String(), e, r, step, hash[:], scope)
	exec("INSERT INTO artifacts(id,result_ref,enterprise_id,conversation_id,run_id,content_type,data_classification,content,authorization_scope,content_hash,byte_size) VALUES($1,$2,$3,$4,$5,'application/json','internal',$6,$7,$8,$9)", artifact, ref, e, c, r, body, scope, hash[:], len(body))
	exec("INSERT INTO tool_results(id,tool_call_id,enterprise_id,artifact_id,projection,projection_hash,projection_bytes) VALUES($1,$2,$3,$4,'{}',$5,2)", uuid.New(), call, e, artifact, hash[:])
	// The durable metadata is exactly the source tracking used by ImportResult.
	exec("INSERT INTO workspace_files(id,enterprise_id,workspace_id,conversation_id,name,path,byte_size,content_hash,media_type,source_result_refs) VALUES($1,$2,$3,$4,'business.json','business.json',$5,$6,'application/json',$7)", file, e, w, c, len(body), fmt.Sprintf("%x", hash), []string{ref})
	if _, err := (conversation.Service{Store: store}).ReadToolResult(ctx, e, u, ref); err != nil {
		t.Fatalf("authorized fixture is invalid: %v", err)
	}
	exec("UPDATE workspaces SET source_result_refs=$2 WHERE id=$1", w, []string{ref})
	exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", u)
	actor, err := store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: u, EnterpriseID: e})
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE runs SET status='succeeded' WHERE id=$1", r)
	r = uuid.New()
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',$6,'running')", r, c, e, u, m, actor.AuthorizationVersion)
	p := toolruntime.Principal{EnterpriseID: e, UserID: u, ConversationID: c, AuthorizationVersion: actor.AuthorizationVersion, Permissions: []string{"workspace.use"}}
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	client := fake.NewSimpleClientset(&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "argus-workspace"}, Provisioner: "rawfile.csi.openebs.io", Parameters: map[string]string{"thinProvision": "false", "csi.storage.k8s.io/fstype": "ext4"}, MountOptions: []string{"nodiscard"}, VolumeBindingMode: &mode})
	cfg := config.Workspace{Enabled: true, Namespace: "test-workspace", StorageClass: "argus-workspace", IOImage: "fixed-io"}
	service := Service{Store: store, Config: cfg, Kubernetes: Kubernetes{Client: client, Config: cfg}, Idempotency: postgres.Idempotency{Key: []byte(strings.Repeat("k", 32))}}
	if err := service.Authorize(ctx, p); err != nil {
		t.Fatalf("new principal should retain workspace use: %v", err)
	}
	_, err = (conversation.Service{Store: store}).GetToolResult(ctx, e, u, ref)
	if err == nil {
		t.Fatal("source revocation failed")
	}
	t.Logf("source access rejected after authorization change: %v", err)
	if _, _, _, err := service.ReadFile(ctx, p, file, 0, 0); err == nil {
		t.Fatal("file API unexpectedly accepted source")
	} else {
		t.Logf("file API rejected: %v", err)
	}
	if _, err := service.publicationSources(ctx, p, w); err == nil {
		t.Fatal("publication unexpectedly accepted source")
	} else {
		t.Logf("publication rejected: %v", err)
	}

	for _, metadata := range []bool{true, false} {
		if !metadata {
			exec("DELETE FROM workspace_files WHERE id=$1", file)
		}
		for _, accessScope := range []accessScope{{RunID: r}, {WorkspaceID: w}} {
			if _, err := service.ensure(ctx, p, accessScope); err == nil {
				t.Fatalf("revoked source admitted, metadata=%v", metadata)
			}
			if _, err := service.access(ctx, p, false, uuid.Nil, accessScope); err == nil {
				t.Fatal("file/compute access accepted revoked source")
			}
		}
	}
	if err := service.registerUntrustedSourceForTest(ctx, p, w, ref); err == nil {
		t.Fatal("revoked source registration accepted")
	}
	current, err := store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: w, EnterpriseID: e})
	if err != nil || current.Status != "ready" || len(current.SourceResultRefs) != 1 || current.FenceToken != 0 {
		t.Fatalf("authorization failure mutated the directory: %v", err)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource != "storageclasses" {
			t.Fatal("denied operation reached a workload mutation")
		}
	}
	accessCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	access := &Access{Context: accessCtx, Workspace: current, service: service, principal: &p, cancel: cancel, renewed: make(chan struct{})}
	go access.renew()
	select {
	case <-access.renewed:
	case <-time.After(3 * time.Second):
		t.Fatal("revoked active access was not cancelled")
	}
	if accessCtx.Err() == nil {
		t.Fatal("revocation did not cancel RPC context")
	}
	if _, err := service.Delete(ctx, p, uuid.NewString()); err != nil {
		t.Fatalf("explicit cleanup must remain authorized: %v", err)
	}
}

// Use the same pre-write entry as ImportResult, with no IO client configured.
// Revoked imports must fail before any write or lease registration can occur.
func (service Service) registerUntrustedSourceForTest(ctx context.Context, p toolruntime.Principal, w uuid.UUID, ref string) error {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	a := &Access{Context: c, service: service, principal: &p, cancel: cancel, Workspace: db.Workspace{ID: w, EnterpriseID: p.EnterpriseID}}
	err := a.registerSource(ctx, ref)
	var failure toolruntime.Error
	if err == nil || !errors.As(err, &failure) || failure.Kind != "TOOL_RESULT_FORBIDDEN" {
		return nil
	}
	return err
}
