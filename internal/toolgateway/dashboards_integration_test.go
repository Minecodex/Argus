package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type dashboardExternalFixture struct{}

type dashboardContextBackend struct{}

func (dashboardContextBackend) ExecuteEngineQuery(context.Context, queryengine.Request) (queryengine.Result, error) {
	return queryengine.Result{}, errors.New("no panels in fixture")
}
func (dashboardContextBackend) DiscoverData(context.Context, telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	return telemetry.DataCatalogResult{}, errors.New("no sources in fixture")
}

type dashboardContextObjects struct{}

func (dashboardContextObjects) PutFile(context.Context, string, io.Reader, int64, string) error {
	return errors.New("not materialized by admission test")
}
func (dashboardContextObjects) ReadFile(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return nil, errors.New("not materialized by admission test")
}

func (dashboardExternalFixture) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	return toolruntime.Contribution{Tools: []toolruntime.Tool{{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "customer_query", Schema: emptyObjectSchema()}, Source: "external_mcp", Version: "1"}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
		return toolruntime.Result{}, nil
	}}}}, nil
}
func TestPostgresDashboardChatSelectionAndTools(t *testing.T) {
	address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
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
	e, d, u, m, c, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Dashboard Chat',$2,'UTC')", e, e.String())
	defer func() {
		_, _ = store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", e)
	}()
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Chat')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Chat')", u, e, d, u.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Chat')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read'),($1,'telemetry.dashboard.manage'),($1,'workspace.use')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Chat','https://model.example.test','model','chat_completions',1000000,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Chat',$4)", c, e, u, m)
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	spec, _ := json.Marshal(dashboard.EmptySpec())
	a, b := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{a, b} {
		rev := uuid.New()
		exec("INSERT INTO dashboards(id,enterprise_id,name,created_by,updated_by) VALUES($1,$2,'Selected dashboard',$3,$3)", id, e, u)
		exec("INSERT INTO dashboard_revisions(id,enterprise_id,dashboard_id,revision_number,schema_version,name,description,spec,spec_hash,validation_report,sample_report,created_by) VALUES($1,$2,$3,1,'argus.dashboard/v1','Selected dashboard','',$4,'test','{}','{}',$5)", rev, e, id, spec, u)
		exec("UPDATE dashboards SET active_revision_id=$2 WHERE id=$1", id, rev)
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'dashboard',$4)", uuid.New(), e, u, id)
	}
	key := bytes.Repeat([]byte{5}, 32)
	service := dashboard.Service{Store: store, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	registry := mcp.NewRegistry()
	if err = (DashboardTools{Service: service, Jobs: &dashboard.QueryJobs{Runtime: dashboard.Runtime{Store: store, Backend: dashboardContextBackend{}}, Objects: dashboardContextObjects{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	gateway, err := New(registry, "dashboard-chat-test")
	if err != nil {
		t.Fatal(err)
	}
	policy := dashboardcontext.Policy{Store: store}
	gateway.Scope = policy.BusinessScope
	convo := conversation.Service{Store: store, Idempotency: postgres.Idempotency{Key: key}}
	factory := toolruntime.CompositeFactory{Providers: []toolruntime.Provider{gateway, dashboardExternalFixture{}}, ContextSources: []toolruntime.SkillContextSource{policy}, Filter: policy.Filter, AuthorizeTool: policy.AuthorizeTool, Validate: convo.ValidateToolPrincipal}
	convo.Tools = factory
	p, err := convo.ToolPrincipal(ctx, e, u, c)
	if err != nil {
		t.Fatal(err)
	}
	finish := func(run uuid.UUID) {
		exec("UPDATE runs SET status='succeeded' WHERE id=$1", run)
		exec("UPDATE runtime_tasks SET status='succeeded' WHERE run_id=$1", run)
	}
	send := func(sel *dashboardcontext.Selection, content string) (conversation.MessageAccepted, error) {
		return convo.AddMessage(ctx, u.String(), e, u, c, 1, "zh-CN", content, nil, uuid.NewString(), sel)
	}
	general, err := send(nil, "List dashboards")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(general.Run.ToolSnapshot, []byte("customer_query")) {
		t.Fatal("general Run lost direct customer MCP")
	}
	finish(general.Run.ID)
	zero := int64(0)
	selected, err := send(&dashboardcontext.Selection{Mode: "analyze", DashboardIDs: []uuid.UUID{a}, ExpectedVersion: &zero}, "Check this dashboard for the last 30 minutes")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(selected.Run.ToolSnapshot, []byte("customer_query")) || !bytes.Contains(selected.Run.ToolSnapshot, []byte("telemetry.dashboard.analyze")) {
		t.Fatal("analysis snapshot does not isolate tools and skill")
	}
	var snapshot toolruntime.Snapshot
	if err = json.Unmarshal(selected.Run.ToolSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	restored, err := factory.Restore(ctx, p, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(set *toolruntime.Set, run uuid.UUID, name string, input map[string]any) (toolruntime.Result, error) {
		return set.Invoke(ctx, "tool.invoke", toolruntime.Invocation{ID: uuid.New(), RunID: run, Principal: p, Arguments: map[string]any{"category": "dashboard", "name": name, "arguments": input}})
	}
	if _, err = invoke(restored, selected.Run.ID, "get", map[string]any{"dashboard_id": a.String()}); err != nil {
		t.Fatal(err)
	}
	resolution, err := invoke(restored, selected.Run.ID, "context.resolve", map[string]any{"dashboard_id": a.String(), "expected_version": float64(0), "changes": map[string]any{"time": map[string]any{"kind": "relative", "seconds": float64(1800)}}, "evidence": []any{map[string]any{"path": "/time", "quote": "last 30 minutes"}}})
	if err != nil {
		t.Fatal(err)
	}
	ref := resolution.Data["context_ref"].(string)
	if resolution.Data["condition_version"] != float64(1) {
		t.Fatal("explicit condition not persisted")
	}
	job, err := invoke(restored, selected.Run.ID, "query", map[string]any{"dashboard_id": a.String(), "context_ref": ref})
	if err != nil || job.Data["status"] != "queued" {
		t.Fatalf("resolved root query admission: %v", err)
	}
	// Status reads must attach current accounting, not the quota observed when
	// the immutable query was admitted. Exhaust a real persistent budget row.
	budgetBefore, ok := job.Data["run_budget"].(map[string]any)
	if !ok || job.Data["run_budget_observed_at"] == nil {
		t.Fatal("query omitted its current budget snapshot")
	}
	remainingBefore := budgetBefore["remaining"].(map[string]any)["scan_bytes"].(float64)
	if remainingBefore <= 0 {
		t.Fatal("fixture needs initial scan capacity")
	}
	exec("UPDATE dashboard_run_budgets SET scan_remaining=0 WHERE run_id=$1", selected.Run.ID)
	status, err := invoke(restored, selected.Run.ID, "query.get", map[string]any{"job_id": job.Data["id"]})
	if err != nil {
		t.Fatal(err)
	}
	budgetNow := status.Data["run_budget"].(map[string]any)["remaining"].(map[string]any)
	if budgetNow["scan_bytes"] != float64(0) || status.Data["id"] != job.Data["id"] {
		t.Fatal("query status retained stale accounting or changed query identity")
	}
	exec("UPDATE dashboard_run_budgets SET scan_remaining=$2 WHERE run_id=$1", selected.Run.ID, int64(remainingBefore))
	if _, err = invoke(restored, selected.Run.ID, "context.resolve", map[string]any{"dashboard_id": a.String(), "expected_version": float64(1), "changes": map[string]any{"reset_time": true}, "evidence": []any{map[string]any{"path": "/time", "quote": "invented user instruction"}}}); err == nil {
		t.Fatal("invented condition evidence accepted")
	}
	if _, err = store.Pool.Exec(ctx, "UPDATE dashboard_analysis_contexts SET condition_version=condition_version+1 WHERE id=$1", uuid.MustParse(ref)); err == nil {
		t.Fatal("resolved context mutable")
	}
	if _, err = invoke(restored, selected.Run.ID, "get", map[string]any{"dashboard_id": b.String()}); err == nil {
		t.Fatal("model changed selected dashboard")
	}
	if _, err = invoke(restored, selected.Run.ID, "query", map[string]any{"dashboard_id": b.String(), "parameters": map[string]any{}}); err == nil {
		t.Fatal("query bypassed structured selection")
	}
	if _, err = restored.Invoke(ctx, "tool.describe", toolruntime.Invocation{RunID: selected.Run.ID, Principal: p, Arguments: map[string]any{"category": "metric", "name": "query"}}); err == nil {
		t.Fatal("raw query visible in dashboard analysis")
	}
	if _, err = restored.Invoke(ctx, "customer_query", toolruntime.Invocation{RunID: selected.Run.ID, Principal: p, Arguments: map[string]any{}}); err == nil {
		t.Fatal("external MCP escaped analysis")
	}
	if _, err = store.Pool.Exec(ctx, "UPDATE dashboard_run_contexts SET context_version=context_version+1 WHERE run_id=$1", selected.Run.ID); err == nil {
		t.Fatal("Run selection mutable")
	}
	finish(selected.Run.ID)
	follow, err := send(nil, "And this time?")
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := invoke(restored, follow.Run.ID, "context.resolve", map[string]any{"dashboard_id": a.String(), "expected_version": float64(1), "changes": map[string]any{}, "evidence": []any{}})
	if err != nil || inherited.Data["condition_version"] != float64(1) {
		t.Fatalf("follow-up did not inherit: %v", err)
	}
	conditions := inherited.Data["conditions"].(map[string]any)["overrides"].(map[string]any)
	if conditions["time"].(map[string]any)["seconds"] != float64(1800) {
		t.Fatal("explicit time was replaced by default")
	}
	if _, err = invoke(restored, follow.Run.ID, "query", map[string]any{"dashboard_id": a.String(), "context_ref": ref}); err == nil {
		t.Fatal("prior Run context reused")
	}
	newRevision := uuid.New()
	exec("INSERT INTO dashboard_revisions(id,enterprise_id,dashboard_id,revision_number,schema_version,name,description,spec,spec_hash,validation_report,sample_report,created_by) VALUES($1,$2,$3,2,'argus.dashboard/v1','New publication','',$4,'new','{}','{}',$5)", newRevision, e, a, spec, u)
	exec("UPDATE dashboards SET active_revision_id=$2 WHERE id=$1", a, newRevision)
	if _, err = invoke(restored, follow.Run.ID, "query", map[string]any{"dashboard_id": a.String(), "context_ref": inherited.Data["context_ref"]}); !errors.Is(err, dashboard.ErrContextExpired) {
		t.Fatalf("stale revision admitted: %v", err)
	}
	refreshed, err := invoke(restored, follow.Run.ID, "context.resolve", map[string]any{"dashboard_id": a.String(), "expected_version": float64(1), "changes": map[string]any{}, "evidence": []any{}})
	if err != nil || refreshed.Data["revision_id"] != newRevision.String() {
		t.Fatalf("new resolution did not use latest publication: %v", err)
	}
	frozen, err := dashboardcontext.ForRun(ctx, store.Queries, p, follow.Run.ID)
	if err != nil || len(frozen.DashboardIDs) != 1 || frozen.DashboardIDs[0] != a {
		t.Fatalf("follow-up lost explicit IDs: %+v %v", frozen, err)
	}
	finish(follow.Run.ID)
	if _, err = send(&dashboardcontext.Selection{Mode: "analyze", DashboardIDs: []uuid.UUID{b}, ExpectedVersion: &zero}, "stale tab"); err == nil {
		t.Fatal("old tab silently replaced selection")
	}
	one := int64(1)
	second, err := send(&dashboardcontext.Selection{Mode: "analyze", DashboardIDs: []uuid.UUID{b}, ExpectedVersion: &one}, "switch")
	if err != nil {
		t.Fatal(err)
	}
	finish(second.Run.ID)
	// Recovery of the first Run keeps A even after the conversation switches to B.
	frozen, err = dashboardcontext.ForRun(ctx, store.Queries, p, selected.Run.ID)
	if err != nil || frozen.DashboardIDs[0] != a {
		t.Fatal("old Run inherited newer selection")
	}
	forged := p
	forged.UserID = uuid.New()
	if _, err = dashboardcontext.ForRun(ctx, store.Queries, forged, selected.Run.ID); err == nil {
		t.Fatal("foreign owner read Run context")
	}
	exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND resource_id=$2", e, b)
	if _, err = send(nil, "revoked follow-up"); err == nil {
		t.Fatal("revoked dashboard admitted")
	}
	if _, err = invoke(restored, second.Run.ID, "get", map[string]any{"dashboard_id": b.String()}); err == nil {
		t.Fatal("revoked selection continued using frozen tools")
	}
	cleared, err := send(&dashboardcontext.Selection{Mode: "none", DashboardIDs: []uuid.UUID{}}, "leave dashboard mode")
	if err != nil {
		t.Fatal(err)
	}
	finish(cleared.Run.ID)
	created, err := send(nil, "/创建仪表盘 Create a new dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(created.Run.ToolSnapshot, []byte("telemetry.dashboard.create")) {
		t.Fatal("slash command did not activate creation")
	}
	createSet, err := factory.Build(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := invoke(createSet, created.Run.ID, "catalog.resources", map[string]any{"limit": float64(10)})
	if err != nil || len(resources.Data["resources"].([]any)) != 0 || len(resources.Data["source_capabilities"].([]any)) != 0 {
		t.Fatalf("resource catalog crossed enterprise/authorization boundary: %v", err)
	}
	clock, err := time.Parse(time.RFC3339Nano, resources.Data["server_time"].(string))
	if err != nil || time.Since(clock) > 5*time.Second || clock.After(time.Now()) || resources.Data["timezone"] != "UTC" {
		t.Fatalf("creation catalog clock is unavailable or stale: %v", err)
	}
	window := resources.Data["suggested_catalog_range"].(map[string]any)
	from, err := time.Parse(time.RFC3339Nano, window["from"].(string))
	if err != nil {
		t.Fatal(err)
	}
	to, err := time.Parse(time.RFC3339Nano, window["to"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !to.Equal(clock) || to.Sub(from) != time.Hour {
		t.Fatal("suggested catalog range does not match the dashboard default/current server clock")
	}
	input := map[string]any{"name": "AI draft", "description": "", "spec": dashboard.EmptySpec(), "proposed_bindings": []any{}}
	invocation := toolruntime.Invocation{ID: uuid.New(), RunID: created.Run.ID, Principal: p, Arguments: map[string]any{"category": "dashboard", "name": "draft.create", "arguments": input}}
	// Go values must be JSON-shaped just like model inputs.
	raw, _ := json.Marshal(invocation.Arguments)
	_ = json.Unmarshal(raw, &invocation.Arguments)
	draft, err := createSet.Invoke(ctx, "tool.invoke", invocation)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := createSet.Invoke(ctx, "tool.invoke", invocation)
	if err != nil || replay.Data["id"] != draft.Data["id"] {
		t.Fatalf("initial draft replay duplicated: %v", err)
	}
	draftID := draft.Data["id"].(string)
	sampled, err := invoke(createSet, created.Run.ID, "draft.validate", map[string]any{"draft_id": draftID, "expected_version": float64(1)})
	if err != nil || sampled.Data["validation"].(map[string]any)["valid"] != true {
		t.Fatalf("valid draft validation unavailable: %v", err)
	}
	invalidSpec := dashboard.EmptySpec()
	invalidSpec.DefaultRefreshSeconds = 1 // An editable draft may have hard publication errors.
	invalidInput := map[string]any{"name": "Invalid draft", "description": "", "spec": invalidSpec, "proposed_bindings": []any{}}
	rawInvalid, _ := json.Marshal(invalidInput)
	_ = json.Unmarshal(rawInvalid, &invalidInput)
	invalidDraft, err := invoke(createSet, created.Run.ID, "draft.create", invalidInput)
	if err != nil || invalidDraft.Data["validation"].(map[string]any)["valid"] != false {
		t.Fatalf("draft did not expose editable validation issues: %v", err)
	}
	invalidID := invalidDraft.Data["id"].(string)
	sampled, err = invoke(createSet, created.Run.ID, "draft.validate", map[string]any{"draft_id": invalidID, "expected_version": float64(1)})
	if err != nil || sampled.Data["validation"].(map[string]any)["valid"] != false || sampled.Data["sample"].(map[string]any)["status"] != "not_executed" {
		t.Fatalf("hard validation feedback was lost or mistaken for execution: %v", err)
	}
	if _, err = invoke(createSet, created.Run.ID, "draft.validate", map[string]any{"draft_id": invalidID, "expected_version": float64(2)}); !errors.Is(err, dashboard.ErrConflict) {
		t.Fatalf("stale draft validation was accepted: %v", err)
	}
	if _, err = invoke(createSet, created.Run.ID, "publish.preview", map[string]any{"draft_id": invalidID, "expected_version": float64(1)}); !errors.Is(err, dashboard.ErrInvalid) {
		t.Fatalf("invalid configuration reached publication: %v", err)
	}
	preview, err := invoke(createSet, created.Run.ID, "publish.preview", map[string]any{"draft_id": draftID, "expected_version": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ActionRef == "" {
		t.Fatal("preview has no host confirmation")
	}
	action, err := store.Queries.GetPendingAction(ctx, db.GetPendingActionParams{ActionRef: preview.ActionRef, EnterpriseID: e})
	if err != nil || !action.RunID.Valid || action.RunID.UUID != created.Run.ID {
		t.Fatalf("preview not attached to Run: %+v %v", action, err)
	}
	row, err := service.Draft(ctx, dashboard.Actor{EnterpriseID: e, SubjectID: u, SubjectType: "user", AuthorizationVersion: 1}, uuid.MustParse(draftID))
	if err != nil || row.Status != "editing" || row.PublishedRevisionID.Valid {
		t.Fatal("AI directly published draft")
	}
	if _, err = invoke(createSet, created.Run.ID, "publish.commit", map[string]any{}); err == nil {
		t.Fatal("commit visible to model")
	}
	if _, err = invoke(createSet, created.Run.ID, "draft.save", map[string]any{"draft_id": draftID, "name": "Changed", "description": "", "spec": map[string]any{}, "proposed_bindings": []any{}, "expected_version": float64(0)}); err == nil {
		t.Fatal("invalid or stale draft was accepted")
	}
	finish(created.Run.ID)
	edit, err := send(&dashboardcontext.Selection{Mode: "create", DashboardIDs: []uuid.UUID{a}}, "Edit the selected dashboard")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := invoke(createSet, edit.Run.ID, "get", map[string]any{"dashboard_id": a.String()})
	if err != nil || definition.Data["revision_id"] != newRevision.String() {
		t.Fatalf("creation get incorrectly required analysis conditions: %v", err)
	}
	if _, exists := definition.Data["analysis"]; exists {
		t.Fatal("creation unexpectedly exposed an analysis context")
	}
	finish(edit.Run.ID)
	t.Log("structured selection, immutable recovery, stale tabs, revocation, native scope, personal draft replay and host-only publication passed")
}
