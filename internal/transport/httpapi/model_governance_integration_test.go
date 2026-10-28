package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"strings"
)

func TestModelGovernancePostgresClickHouseHTTP(t *testing.T) {
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("requires isolated PostgreSQL and ClickHouse")
	}
	f := newAuditWorkflowFixture(t)
	ctx := t.Context()
	conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", "modelgov_test", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err = modelmetrics.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err = (telemetry.ClickHouseTenantSchemaManager{Conn: conn, Router: telemetry.TenantTableRouter{}}).EnsureTenant(ctx, f.enterprise); err != nil {
		t.Fatal(err)
	}
	readStore, writeStore, readConn, writeConn := modelGovernanceRoleStores(t, f, conn, address, os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	backend := modelGovernanceBackend(t, modelmetrics.Service{Store: readStore, Conn: readConn})
	domain := dashboard.Service{Store: f.store, ProvideDefaults: true}
	if err = domain.SyncTemplates(ctx, dashboard.BuiltinTemplates()); err != nil {
		t.Fatal(err)
	}
	origins := []string{f.server.URL, "http://127.0.0.1:4283"}
	key := bytes.Repeat([]byte{23}, 32)
	auth := SetupHandler{Config: config.Server{AllowedOrigins: origins}, Identity: identity.Service{Store: f.store}}
	enterprise := EnterpriseIdentityHandler{Auth: auth, Service: identity.EnterpriseService{Store: f.store}, Cursor: pagination.Signer{Key: key}}
	handler := DashboardHandler{Identity: enterprise, Service: domain, Runtime: &dashboard.Runtime{Store: f.store, Backend: backend, ContextKey: key}}
	models := ModelHandler{Identity: enterprise, Service: model.Service{Store: f.store, Idempotency: f.domain.Actions.Idempotency}}
	policy := configureModelGovernanceLive(t, &models)
	conversations := ConversationHandler{Identity: enterprise, Service: conversation.Service{Store: f.store, Idempotency: f.domain.Actions.Idempotency, Tools: modelGovernanceTools(t)}}
	hosts := HostHandler{Identity: enterprise, Service: f.domain, Queries: f.store.Queries}
	clusters := KubernetesHandler{Identity: enterprise, Service: f.domain}
	workflow := WorkflowHandler{Identity: enterprise, Service: f.workflow}
	f.server.Config.Handler = NewRouterWithOptions(RouterOptions{Setup: &auth, EnterpriseIdentity: &enterprise, Dashboard: &handler, Model: &models, Conversation: &conversations, Host: &hosts, Kubernetes: &clusters, Workflow: &workflow, AllowedOrigins: origins})
	modelID := uuid.New()
	f.exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Governance acceptance','https://controlled.invalid','controlled','chat_completions',32768,1024,1,2,'healthy')`, modelID, f.enterprise)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
	makeCall := func(user, department uuid.UUID, at time.Time, status, kind, source string, input, output, cached, latency int, amount string) {
		conversation, run, step, call := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		f.exec(`INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Metric acceptance',$4)`, conversation, f.enterprise, user, modelID)
		f.exec(`INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'succeeded')`, run, conversation, f.enterprise, user, modelID)
		f.exec(`INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'model_call','succeeded')`, step, run, f.enterprise)
		f.exec(`INSERT INTO model_calls(id,enterprise_id,run_id,step_id,model_id,model_revision,call_kind,projection_hash,input_price_snapshot,output_price_snapshot,status) VALUES($1,$2,$3,$4,$5,1,$6,decode(repeat('00',32),'hex'),1,2,'reserved')`, call, f.enterprise, run, step, modelID, kind)
		reservation := uuid.New()
		local := at.In(loc)
		billingMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
		f.exec(`INSERT INTO model_quota_reservations(id,enterprise_id,model_call_id,model_id,department_id,user_id,month,reserved_amount,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,1,'active',now()+interval '1 hour')`, reservation, f.enterprise, call, modelID, department, user, billingMonth)
		cacheSource := "provider"
		if source != "provider" {
			cacheSource = "missing"
		}
		f.exec(`UPDATE model_calls SET input_tokens=$2,output_tokens=$3,cached_input_tokens=$4,input_usage_source=$5,output_usage_source=$5,cached_input_usage_source=$6,amount=$7,latency_ms=$8,status=$9,dispatched_at=$10::timestamptz-interval '2 seconds',completed_at=$10 WHERE id=$1`, call, input, output, cached, source, cacheSource, amount, latency, status, at)
		f.exec(`UPDATE model_quota_reservations SET status='settled',settled_amount=$2 WHERE id=$1`, reservation, amount)
	}
	makeCall(f.user, f.department, month.Add(time.Hour), "succeeded", "inference", "provider", 100, 20, 10, 1000, "1.2")
	makeCall(f.user, f.department, month.AddDate(0, 0, 20), "failed", "inference", "estimated", 30, 5, 0, 2000, "0.2")
	makeCall(f.user, f.department, month.AddDate(0, 1, 0).Add(-time.Microsecond), "succeeded", "compaction", "provider", 40, 10, 4, 500, "0.4")
	deptB, userB := uuid.New(), uuid.New()
	f.exec(`INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Isolated department')`, deptB, f.enterprise)
	f.exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'other-user','Other user')`, userB, f.enterprise, deptB)
	makeCall(userB, deptB, month.AddDate(0, 0, 1), "succeeded", "inference", "provider", 900, 100, 0, 3000, "9")
	makeCall(f.user, f.department, month.Add(-time.Microsecond), "succeeded", "inference", "provider", 777, 7, 0, 999, "4")
	f.exec(`INSERT INTO model_quotas(id,enterprise_id,model_id,subject_type,subject_id,monthly_amount) VALUES($1,$2,$3,'department',$4,20)`, uuid.New(), f.enterprise, modelID, f.department)
	id := modelmetrics.DashboardID(f.enterprise)
	path := "/dashboards/" + id.String()
	body := map[string]any{"resource_ids": []string{}, "variables": map[string]any{"month": map[string]any{"all": false, "values": []string{month.Format("2006-01")}}}, "local_values": map[string]any{}}
	f.call("GET", "/dashboards", nil, 200, "")
	f.call("GET", path, nil, 200, "")
	execute := func() dashboard.Execution {
		var e dashboard.Execution
		raw := f.call("POST", path+"/execute", body, 200, "")
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	if before := execute(); before.ModelGovernance == nil || before.ModelGovernance.Pending < 1 {
		t.Fatalf("pending projection not exposed: %+v", before.ModelGovernance)
	}
	projector := modelmetrics.Projector{Store: writeStore, Conn: writeConn}
	for {
		n, e := projector.Batch(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			break
		}
	}
	e := execute()
	assertModelMetric(t, e, "requests", 4)
	assertModelMetric(t, e, "tokens", 1170)
	assertModelMetric(t, e, "amount", 10.8)
	assertModelMetric(t, e, "success", .75)
	assertModelMetric(t, e, "errors", 1)
	assertModelMetric(t, e, "compaction", 1)
	assertModelMetric(t, e, "quota", .09)
	// Even a trusted public-query request naming a private metric cannot reach
	// the private projection tables; ordinary discovery is empty as well.
	publicScope := queryengine.Scope{EnterpriseID: f.enterprise, SubjectID: f.user, SubjectType: "user", AuthorizationVersion: 1, Resources: []resourceref.Ref{{Type: "host", ID: f.host}}, ResourceIDs: []uuid.UUID{f.host}, SourceKeys: []string{uuid.NewString() + ":1"}, Signature: strings.Repeat("a", 64)}
	publicResult, err := backend.ExecuteEngineQuery(ctx, queryengine.Request{Language: queryengine.LanguagePromQL, Expression: "sum(argus_model_requests)", Instant: true, Start: month.UTC(), End: month.Add(time.Hour).UTC(), Scope: publicScope})
	if err != nil {
		t.Fatal(err)
	}
	publicData, _ := json.Marshal(publicResult.Data)
	if string(publicData) != "[]" {
		t.Fatalf("private metric escaped into public PromQL: %s", publicData)
	}
	publicCatalog, err := backend.DiscoverData(ctx, telemetry.DataCatalogRequest{EnterpriseID: f.enterprise, SubjectID: f.user, SubjectType: "user", AuthorizationVersion: 1, Resources: publicScope.Resources, ResourceIDs: publicScope.ResourceIDs, SourceKeys: publicScope.SourceKeys, ScopeSignature: publicScope.Signature, Signal: "metrics", Kind: "metrics", From: month.UTC(), To: month.Add(time.Hour).UTC(), Limit: 50})
	if err != nil || len(publicCatalog.Metrics) != 0 {
		t.Fatalf("private metrics appeared in public catalog: %v %v", publicCatalog.Metrics, err)
	}
	if _, err = (telemetry.TenantTableRouter{}).Table("argus_internal_model_calls_v1", f.enterprise); err == nil {
		t.Fatal("private table exposed through public router")
	}
	if e.To.Sub(e.From) <= 7*24*time.Hour || e.ModelGovernance.Pending != 0 || e.ModelGovernance.UsageComplete {
		t.Fatal("month/quality status incorrect")
	}
	f.exec(`UPDATE model_metric_outbox SET delivered_at=NULL`)
	if _, err = projector.Batch(ctx); err != nil {
		t.Fatal(err)
	}
	assertModelMetric(t, execute(), "requests", 4)
	// Internal definitions cannot enter generic drafts, even for full administrators.
	var detail struct {
		Revision struct {
			Spec json.RawMessage `json:"spec"`
		} `json:"revision"`
	}
	_ = json.Unmarshal(f.call("GET", path, nil, 200, ""), &detail)
	f.call("POST", "/dashboard-drafts", map[string]any{"dashboard_id": id, "name": "forbidden copy", "description": "", "spec": json.RawMessage(detail.Revision.Spec), "proposed_bindings": []any{}}, 403, "")
	f.call("POST", "/dashboard-drafts", map[string]any{"name": "forbidden source", "description": "", "spec": json.RawMessage(detail.Revision.Spec), "proposed_bindings": []any{}}, 403, "")
	// Department users have no telemetry.dashboard.read and still see their own builtin.
	adminUser, adminDept := f.user, f.department
	reader := uuid.New()
	f.exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'department-reader','Department reader')`, reader, f.enterprise, f.department)
	role := uuid.New()
	f.exec(`INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Model reader')`, role, f.enterprise)
	f.exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'model.usage.read'),($1,'model.quota.manage')`, role)
	f.exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)`, uuid.New(), f.enterprise, reader, role)
	f.user = reader
	f.session()
	f.call("GET", "/dashboards", nil, 200, "")
	departmentResult := execute()
	assertModelMetric(t, departmentResult, "requests", 3)
	assertModelMetric(t, departmentResult, "amount", 1.8)
	raw := f.call("GET", "/enterprise/model-usage?month="+month.Format("2006-01"), nil, 200, "")
	if bytes.Contains(raw, []byte("10.8")) {
		t.Fatal("legacy usage leaked other department")
	}
	f.call("POST", "/enterprise/model-quotas", map[string]any{"model_id": modelID, "subject_type": "user", "subject_id": userB, "monthly_amount": 1}, 403, uuid.NewString())
	f.exec(`DELETE FROM role_permissions WHERE role_id=$1`, role)
	f.exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read')`, role)
	f.session()
	listed := f.call("GET", "/dashboards", nil, 200, "")
	if bytes.Contains(listed, []byte(id.String())) {
		t.Fatal("ordinary dashboard read revealed model governance")
	}
	f.call("GET", path, nil, 403, "")
	f.call("POST", path+"/execute", body, 403, "")
	f.user, f.department = adminUser, adminDept
	f.session()
	foreignEnterprise, foreignModel := uuid.New(), uuid.New()
	f.exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Other model tenant',$2,'UTC')`, foreignEnterprise, foreignEnterprise.String())
	f.exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Private tenant canary','https://controlled.invalid','controlled','chat_completions',32768,1024,1,2,'healthy')`, foreignModel, foreignEnterprise)
	f.call("POST", path+"/execute", map[string]any{"resource_ids": []string{}, "local_values": map[string]any{}, "variables": map[string]any{"model": map[string]any{"all": false, "values": []string{foreignModel.String()}}}}, 403, "")
	f.call("GET", "/enterprise/model-usage?model_id="+foreignModel.String(), nil, 403, "")
	liveModel, liveMonth := runModelGovernanceLive(t, f, models, policy, projector)
	if os.Getenv("ARGUS_MODEL_GOVERNANCE_BROWSER") == "1" {
		if liveModel != uuid.Nil {
			runModelGovernanceBrowser(t, f, id, liveModel, liveMonth)
		} else {
			runModelGovernanceBrowser(t, f, id, modelID, month.Format("2006-01"))
		}
	}
	t.Log("controlled facts: PostgreSQL outbox -> ClickHouse projection -> mTLS metric service -> HTTP Dashboard; month, replay, authorization and copy isolation passed")
}

func assertModelMetric(t *testing.T, e dashboard.Execution, id string, want float64) {
	t.Helper()
	for _, panel := range e.Panels {
		if panel.ID != id {
			continue
		}
		raw, _ := json.Marshal(panel.Targets[0].Data)
		var points []struct {
			Value []any `json:"value"`
		}
		if json.Unmarshal(raw, &points) != nil || len(points) != 1 || len(points[0].Value) != 2 {
			t.Fatalf("%s result %s", id, raw)
		}
		var got float64
		switch value := points[0].Value[1].(type) {
		case string:
			_, _ = fmt.Sscan(value, &got)
		case float64:
			got = value
		}
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("%s = %v want %v", id, got, want)
		}
		return
	}
	t.Fatal("missing metric", id)
}
func runModelGovernanceBrowser(t *testing.T, f *auditWorkflowFixture, id, model uuid.UUID, month string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model-governance.json")
	raw, _ := json.Marshal(map[string]any{"api": f.server.URL + "/api/v1", "token": f.token, "csrf": f.csrf, "dashboardId": id, "modelId": model, "month": month})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	command := exec.CommandContext(t.Context(), "node", filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "--config", "playwright.model-governance.config.ts", "--workers=1")
	command.Dir = filepath.Join(root, "web", "apps", "enterprise")
	command.Env = append(os.Environ(), "ARGUS_MODEL_GOVERNANCE_FIXTURE="+path, "VITE_API_BASE_URL="+f.server.URL)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
