package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/connector"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestLifecycleGatewayPreparesPublicTemplateAndKeepsRunForExecutor(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e, d, u, role, model, convo, run, scope, host := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	defer func() {
		_, _ = store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", e)
	}()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Preview gateway',$2,'UTC')", e, "p5-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Preview user')", u, e, d, "p5-"+u.String())
	exec("INSERT INTO roles(id,enterprise_id,name,builtin) VALUES($1,$2,'Preview role',false)", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'bastion_scope.manage')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'P5',$4)", convo, e, u, model)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", run, convo, e, u, model)
	exec("INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,'Pending bastion','development',decode(repeat('00',32),'hex'),'direct_install','pending')", scope, e)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status) VALUES($1,$2,'Pending bastion','192.0.2.40',22,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','direct',$3,'active')", host, e, scope)
	exec("UPDATE bastion_scopes SET connector_host_id=$2 WHERE id=$1", scope, host)
	if err := audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}, Key: bytes.Repeat([]byte{4}, 32)}
	base := ResourceTools{Store: store, Resources: resource.Service{Store: store, Actions: actions}}
	registry := mcp.NewRegistry()
	if err := (LifecycleTools{Base: base, Bastion: connector.BastionService{Store: store, Actions: actions}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	gateway, err := New(registry, "preview-integration")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	principal := toolruntime.Principal{EnterpriseID: e, UserID: u, ConversationID: convo, Permissions: []string{"bastion_scope.manage"}}
	refs := []string{}
	for _, name := range []string{"bastion.update.preview", "bastion.delete.preview"} {
		call := toolruntime.Invocation{ID: uuid.New(), RunID: run, Principal: principal, Arguments: map[string]any{"category": "connector", "name": name, "arguments": map[string]any{"scope_id": scope.String(), "expected_version": float64(1)}}}
		result, err := set.Invoke(t.Context(), "tool.invoke", call)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.ActionRef == "" || result.Presentation == nil || result.Presentation.Hash == "" || result.Data["preview"] == nil {
			t.Fatalf("%s missing public details/host action/template", name)
		}
		refs = append(refs, result.ActionRef)
		var linked uuid.UUID
		if err := store.Pool.QueryRow(t.Context(), "SELECT run_id FROM pending_actions WHERE action_ref=$1", result.ActionRef).Scan(&linked); err != nil || linked != run {
			t.Fatalf("executor cannot resume originating Run: %v", err)
		}
		encoded, _ := json.Marshal(result)
		for _, private := range []string{"immutable_plan", "commit_token", "plan_hash", "resource_scope_snapshot"} {
			if strings.Contains(string(encoded), private) {
				t.Fatalf("private %s leaked through result/template", private)
			}
		}
		// A still-authorized Run cannot bypass a later authoritative revocation.
		exec("UPDATE role_bindings SET status='disabled' WHERE role_id=$1", role)
		call.ID = uuid.New()
		if _, err := set.Invoke(t.Context(), "tool.invoke", call); err == nil {
			t.Fatal("revoked caller prepared another action")
		}
		exec("UPDATE role_bindings SET status='active' WHERE role_id=$1", role)
	}
	exec("UPDATE runs SET status='waiting_input',stop_reason='pending_action_confirmation' WHERE id=$1", run)
	if _, err := conversation.AppendEvent(t.Context(), store.Queries, conversation.EventInput{EnterpriseID: e, ConversationID: convo, RunID: uuid.NullUUID{UUID: run, Valid: true}, Type: "pending_action_created", ActorType: "system", Payload: map[string]any{"action_ref": refs[0]}, Classification: "internal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.Cancel(t.Context(), u.String(), e, refs[1], uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	current, err := store.Queries.GetRun(t.Context(), db.GetRunParams{ID: run, EnterpriseID: e})
	if err != nil || current.Status != "waiting_input" {
		t.Fatalf("unrelated action cancellation stopped this Run: %v", err)
	}
	key := uuid.NewString()
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := actions.Cancel(t.Context(), u.String(), e, refs[0], key); err != nil {
			t.Fatal(err)
		}
	}
	current, err = store.Queries.GetRun(t.Context(), db.GetRunParams{ID: run, EnterpriseID: e})
	if err != nil || current.Status != "cancelled" || current.StopReason.String != "pending_action_cancelled" {
		t.Fatalf("one Preview cancellation left its Run waiting: %v", err)
	}
	var terminalEvents int
	if err := store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='cancelled'", run).Scan(&terminalEvents); err != nil || terminalEvents != 1 {
		t.Fatalf("cancel terminal events = %d / %v", terminalEvents, err)
	}
}
