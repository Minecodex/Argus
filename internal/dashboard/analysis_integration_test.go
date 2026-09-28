package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type conditionTools struct{}

func (conditionTools) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	result := toolruntime.Contribution{NativeCatalog: "conditions-test"}
	for _, name := range []string{"tool.search", "tool.describe", "tool.invoke"} {
		result.Tools = append(result.Tools, toolruntime.Tool{Definition: toolruntime.Definition{Source: "argus", Version: "1", Model: modelprovider.Tool{Name: name, Schema: map[string]any{"type": "object"}}}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			return toolruntime.Result{}, nil
		}})
	}
	return result, nil
}

type analysisFixture struct {
	store              *postgres.Store
	actor              Actor
	conversation, a, b uuid.UUID
	service            Service
	convo              conversation.Service
}

func newAnalysisFixture(t *testing.T) analysisFixture {
	t.Helper()
	address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	e, d, u, m, c, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fixture := analysisFixture{store: store, actor: Actor{EnterpriseID: e, SubjectID: u, SubjectType: "user", AuthorizationVersion: 1}, conversation: c, a: uuid.New(), b: uuid.New()}
	t.Cleanup(func() {
		_, _ = store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", e)
		store.Close()
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Conditions',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Conditions')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Conditions')", u, e, d, u.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Conditions')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read'),($1,'telemetry.dashboard.manage'),($1,'workspace.use')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Conditions','https://model.example.test','model','chat_completions',1000000,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Conditions',$4)", c, e, u, m)
	for _, id := range []uuid.UUID{fixture.a, fixture.b} {
		exec("INSERT INTO dashboards(id,enterprise_id,name,created_by,updated_by) VALUES($1,$2,'Conditions',$3,$3)", id, e, u)
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'dashboard',$4)", uuid.New(), e, u, id)
	}
	fixture.publish(t, fixture.a, 1, conditionFixture())
	second := EmptySpec()
	second.DefaultTimeRange.Seconds = 7200
	fixture.publish(t, fixture.b, 1, second)
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", nullID(e)); err != nil {
		t.Fatal(err)
	}
	fixture.service = Service{Store: store}
	fixture.convo = conversation.Service{Store: store, Tools: toolruntime.CompositeFactory{Providers: []toolruntime.Provider{conditionTools{}}}, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{8}, 32)}}
	return fixture
}
func (f analysisFixture) publish(t *testing.T, id uuid.UUID, number int, spec Spec) uuid.UUID {
	t.Helper()
	rev := uuid.New()
	raw, _ := json.Marshal(spec)
	_, err := f.store.Pool.Exec(t.Context(), "INSERT INTO dashboard_revisions(id,enterprise_id,dashboard_id,revision_number,schema_version,name,description,spec,spec_hash,validation_report,sample_report,created_by) VALUES($1,$2,$3,$4,'argus.telemetry_dashboard/v1','Conditions','',$5,'test','{}','{}',$6)", rev, f.actor.EnterpriseID, id, number, raw, f.actor.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Pool.Exec(t.Context(), "UPDATE dashboards SET active_revision_id=$2 WHERE id=$1", id, rev); err != nil {
		t.Fatal(err)
	}
	return rev
}
func (f *analysisFixture) begin(t *testing.T, message string) {
	t.Helper()
	if f.actor.RunID != uuid.Nil {
		if _, err := f.store.Pool.Exec(t.Context(), "UPDATE runs SET status='succeeded' WHERE id=$1", f.actor.RunID); err != nil {
			t.Fatal(err)
		}
	}
	accepted, err := f.convo.AddMessage(t.Context(), f.actor.SubjectID.String(), f.actor.EnterpriseID, f.actor.SubjectID, f.conversation, 1, "zh-CN", message, nil, uuid.NewString(), &dashboardcontext.Selection{Mode: "analyze", DashboardIDs: []uuid.UUID{f.a, f.b}})
	if err != nil {
		t.Fatal(err)
	}
	f.actor.RunID = accepted.Run.ID
}

type conditionBackend struct{}

func (conditionBackend) ExecuteEngineQuery(context.Context, queryengine.Request) (queryengine.Result, error) {
	return queryengine.Result{}, errors.New("no panels expected")
}
func (conditionBackend) DiscoverData(context.Context, telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	return telemetry.DataCatalogResult{}, errors.New("no sources expected")
}

type conditionObjects struct{}

func (conditionObjects) PutFile(context.Context, string, io.Reader, int64, string) error {
	return errors.New("no worker in context test")
}
func (conditionObjects) ReadFile(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return nil, errors.New("no worker in context test")
}

func TestPostgresAnalysisConditionsResetCompatibilityAndRunIsolation(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "检查 prod 最近30分钟")
	ctx := t.Context()
	chosen := Selection{Values: []string{"prod"}}
	input := AnalysisResolveInput{DashboardID: f.a, ExpectedVersion: 0, Changes: dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 1800}, Variables: map[string]*Selection{"env": &chosen}}, Evidence: []dashboardparams.Evidence{{Path: "/time", Quote: "最近30分钟"}, {Path: "/variables/env", Quote: "prod"}}}
	invocation := uuid.New()
	first, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, invocation)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, invocation)
	if err != nil || first.ID != replay.ID || first.ConditionVersion != 1 {
		t.Fatal("resolution replay changed context")
	}
	jobs := QueryJobs{Runtime: Runtime{Store: f.store, Backend: conditionBackend{}}, Objects: conditionObjects{}}
	candidates, err := jobs.Runtime.AnalysisCandidates(ctx, f.actor, f.conversation, AnalysisCandidateInput{DashboardID: f.a, ContextRef: first.ID, Name: "env"})
	if err != nil || candidates["would_reset"] != true || candidates["preview_only"] != true {
		t.Fatalf("published candidates unavailable: %v", err)
	}
	beforeQuery, err := f.store.Queries.GetDashboardParameterState(ctx, db.GetDashboardParameterStateParams{ConversationID: f.conversation, DashboardID: f.a, EnterpriseID: f.actor.EnterpriseID, OwnerUserID: f.actor.SubjectID})
	if err != nil || beforeQuery.Version != 1 {
		t.Fatal("candidate preview mutated persistent conditions")
	}
	if _, err = jobs.Runtime.AnalysisCandidates(ctx, f.actor, f.conversation, AnalysisCandidateInput{DashboardID: f.a, ContextRef: first.ID, Name: "unpublished"}); err == nil {
		t.Fatal("unpublished candidate accepted")
	}
	// No authorized sources produces a complete empty candidate set. Only the
	// explicit env selection is reconciled; neither published defaults nor time
	// endpoints are copied into the persistent override record.
	job, err := jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.a, ContextRef: first.ID}, "root")
	if err != nil {
		t.Fatal(err)
	}
	again, err := jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.a, ContextRef: first.ID}, "root")
	if err != nil || again.ID != job.ID {
		t.Fatalf("query replay rejected after candidate reset: %v", err)
	}
	if _, err = jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.a, ContextRef: first.ID}, "new-query-old-context"); !errors.Is(err, ErrContextExpired) {
		t.Fatalf("stale condition version allowed: %v", err)
	}
	state, err := f.store.Queries.GetDashboardParameterState(ctx, db.GetDashboardParameterStateParams{ConversationID: f.conversation, DashboardID: f.a, EnterpriseID: f.actor.EnterpriseID, OwnerUserID: f.actor.SubjectID})
	if err != nil || state.Version != 2 {
		t.Fatalf("candidate reset was not persisted: %v", err)
	}
	decoded, err := decodeConditions(state.State)
	if err != nil || !decoded.Overrides.Variables["env"].All || decoded.Evidence["/variables/env"].Origin != "candidate_reset" || decoded.Overrides.Time.Kind != "relative" {
		t.Fatal("effective reset and relative intent lost")
	}
	b, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.b, ExpectedVersion: 0}, uuid.New())
	if err != nil || b.Parameters.To.Sub(*b.Parameters.From).Hours() != 2 || b.Conditions.Overrides.Time != nil {
		t.Fatal("one dashboard inherited another's defaults or overrides")
	}
	oldRun := f.actor.RunID
	f.begin(t, "继续")
	inherited, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.a, ExpectedVersion: 2}, uuid.New())
	if err != nil || !inherited.Conditions.Overrides.Variables["env"].All || inherited.Parameters.To.Sub(*inherited.Parameters.From).Minutes() != 30 {
		t.Fatalf("follow-up lost effective conditions: %v", err)
	}
	oldActor := f.actor
	oldActor.RunID = oldRun
	if _, err = f.service.ResolveAnalysis(ctx, oldActor, f.conversation, AnalysisResolveInput{DashboardID: f.a, ExpectedVersion: 2}, uuid.New()); !errors.Is(err, ErrContextExpired) {
		t.Fatal("finished Run altered later context")
	}
	changed := conditionFixture()
	changed.Variables[0].Query.Field = "service_name"
	// The old environment field and the new service field share the same display
	// name but represent different conditions, so a plain follow-up must stop.
	f.publish(t, f.a, 2, changed)
	_, err = f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.a, ExpectedVersion: 2}, uuid.New())
	var code toolruntime.Error
	if !errors.As(err, &code) || code.Kind != "DASHBOARD_CONDITIONS_INCOMPATIBLE" {
		t.Fatalf("semantic change did not require user clarification: %v", err)
	}
	f.begin(t, "env 恢复默认")
	reset, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.a, ExpectedVersion: 2, Changes: dashboardparams.Patch{Variables: map[string]*Selection{"env": nil}}, Evidence: []dashboardparams.Evidence{{Path: "/variables/env", Quote: "env 恢复默认"}}}, uuid.New())
	if err != nil || len(reset.Conditions.Overrides.Variables) != 0 || reset.Conditions.Overrides.Time == nil {
		t.Fatalf("selective reset failed: %v", err)
	}
	foreign := f.actor
	foreign.SubjectID = uuid.New()
	if _, err = f.service.ResolveAnalysis(ctx, foreign, f.conversation, AnalysisResolveInput{DashboardID: f.a}, uuid.New()); err == nil {
		t.Fatal("foreign user read/wrote conditions")
	}
}

func TestPostgresAnalysisResourceDirectoryPaginatesBeyondExecutionScopeLimit(t *testing.T) {
	f := newAnalysisFixture(t)
	// The execution limit remains 1000 resources. A directory with 1001 grants
	// must still let a user find and explicitly choose a smaller authorized set.
	ids := []uuid.UUID{}
	for i := 0; i < 1001; i++ {
		ids = append(ids, uuid.New())
	}
	if _, err := f.store.Pool.Exec(t.Context(), "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) SELECT id,$1,'Scoped host-'||id::text,'127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex') FROM unnest($2::uuid[]) AS id", f.actor.EnterpriseID, ids); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Pool.Exec(t.Context(), "INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) SELECT gen_random_uuid(),$1,'user',$2,'host',id FROM unnest($3::uuid[]) AS id", f.actor.EnterpriseID, f.actor.SubjectID, ids); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.CreationCatalog(t.Context(), f.actor, 0, 10)
	if err != nil || len(page["resources"].([]map[string]any)) != 10 || page["has_more"] != true {
		t.Fatalf("large directory cannot be narrowed: %v", err)
	}
	tail, err := f.service.CreationCatalog(t.Context(), f.actor, 1000, 10)
	if err != nil || len(tail["resources"].([]map[string]any)) != 1 || tail["has_more"] != false {
		t.Fatalf("directory tail is incomplete: %v", err)
	}
	if _, err = resolveResources(t.Context(), f.store.Queries, f.actor, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("directory pagination increased the execution scope limit")
	}
}
