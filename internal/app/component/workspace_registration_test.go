package component

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Registered tools capture a Service value. Remote provenance must already be
// configured by the shared constructor, before registration captures that copy.
func TestRegisteredWorkspacePublicationUsesRemoteSourceAuthorization(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated PostgreSQL")
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
	e, d, u, m, c, r, w, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Publication regression',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Publisher')", u, e, d, u.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Publisher')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Test','https://model.example.test','test','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Publication',$4)", c, e, u, m)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", r, c, e, u, m)
	ref := "remote:" + uuid.NewString()
	exec("INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version,source_result_refs) VALUES($1,$2,$3,$4,'test-publication','ready',2147483648,'test',$5)", w, e, c, "workspace-"+w.String(), []string{ref})

	objects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead {
			t.Error("denied publication reached object storage")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer objects.Close()
	t.Setenv("ARGUS_WORKSPACE_ENABLED", "false")
	denied := toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	checks := 0
	service, err := Workspace(ctx, config.Server{ObjectStoreURL: objects.URL, ObjectStoreAccess: "test", ObjectStoreSecret: "test"}, store, postgres.Idempotency{}, sandbox.Service{}, func(_ context.Context, p toolruntime.Principal, source string) (bool, error) {
		if !strings.HasPrefix(source, "remote:") {
			return false, nil
		}
		checks++
		if p.UserID != u || p.ConversationID != c || source != ref {
			t.Fatal("publication lost its source or principal")
		}
		return true, denied
	})
	if err != nil {
		t.Fatal(err)
	}
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	service.Config.Enabled = true
	service.Config.IOImage = "test-io"
	client := fake.NewSimpleClientset(&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: service.Config.StorageClass}, Provisioner: "rawfile.csi.openebs.io", Parameters: map[string]string{"thinProvision": "false", "csi.storage.k8s.io/fstype": "ext4"}, MountOptions: []string{"nodiscard"}, VolumeBindingMode: &mode})
	service.Kubernetes = workspace.Kubernetes{Client: client, Config: service.Config}
	registry := mcp.NewRegistry()
	if err := service.RegisterTools(registry); err != nil {
		t.Fatal(err)
	}
	_, err = registry.Call(ctx, mcp.Call{ToolID: "workflow.publish_file", Enterprise: e.String(), Subject: u.String(), RunID: r.String(), Input: map[string]any{"path": "derived-report.md"}})
	var typed toolruntime.Error
	if checks == 0 || !errors.As(err, &typed) || typed.Kind != denied.Kind {
		t.Fatalf("registered publication bypassed the configured remote source gate: checks=%d error=%v", checks, err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "get" || action.GetResource().Resource != "storageclasses" {
			t.Fatal("denied source reached Workspace workload mutation")
		}
	}
}
