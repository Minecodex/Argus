package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type alertFixture struct {
	t        *testing.T
	s        Service
	a        Actor
	host, sa uuid.UUID
}

func newAlertFixture(t *testing.T) alertFixture {
	t.Helper()
	address := os.Getenv("ARGUS_ALERT_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable Plan6 PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	e, d, u, role, sa, h := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := alertFixture{t: t, s: Service{Store: store}, a: Actor{EnterpriseID: e, SubjectID: u, SubjectType: "user", AuthorizationVersion: 1}, host: h, sa: sa}
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan6',$2,'UTC')", e, e.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Plan6')", d, e)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Operator')", u, e, d, u.String())
	f.exec("INSERT INTO service_accounts(id,enterprise_id,name) VALUES($1,$2,'Evaluator')", sa, e)
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Plan6')", role, e)
	for id, description := range authorization.PermissionRegistry {
		f.exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions WHERE id IN ('alert.read','alert.policy.manage','alert.silence.manage','alert.notification.manage','alert.handle','service_account.read','service_account.manage','host.read','credential.use','pending_action.confirm')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4),($5,$2,'service_account',$6,$4)", uuid.New(), e, u, role, uuid.New(), sa)
	f.addHost(h, "web-prod-01")
	f.grant("user", u, h)
	f.grant("service_account", sa, h)
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", nullID(e)); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{4}, 32)
	f.s.Actions = resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}
	raw, _ := json.Marshal(SettingsSpec{EvaluatorID: sa})
	if _, err = store.Queries.CreateAlertConfiguration(t.Context(), db.CreateAlertConfigurationParams{ID: uuid.New(), EnterpriseID: e, Kind: "settings", Name: "Evaluator", Spec: raw, CreatedBy: u}); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f alertFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, e := f.s.Store.Pool.Exec(f.t.Context(), sql, args...); e != nil {
		f.t.Fatal(e)
	}
}
func (f alertFixture) addHost(id uuid.UUID, name string) {
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels,labels_hash) VALUES($1,$2,$3,'192.0.2.1',22,'linux','managed_host','direct','production','{\"env\":\"production\"}',decode(repeat('00',32),'hex'))", id, f.a.EnterpriseID, name)
}
func (f alertFixture) grant(kind string, subject, host uuid.UUID) {
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,$3,$4,'host',$5)", uuid.New(), f.a.EnterpriseID, kind, subject, host)
}
func (f alertFixture) policy() PolicySpec {
	return PolicySpec{Enabled: true, Resources: ResourceSelector{Mode: "dynamic", Types: []string{"host"}, Matchers: []Matcher{{Key: "env", Operator: "=", Value: "production"}}}, Rules: []Rule{{ID: uuid.New(), Name: "CPU", Expression: "host_cpu_usage_percent", QueryMode: "numeric", Operator: ">", Threshold: 85, RecoveryThreshold: 75, Severity: "critical", Unit: "%"}, {ID: uuid.New(), Name: "Memory", Expression: "host_memory_usage_percent", QueryMode: "numeric", Operator: ">", Threshold: 80, RecoveryThreshold: 70, Severity: "warning", Unit: "%"}}, Mode: "independent", IntervalSeconds: 30, ForSeconds: 0, RecoverySeconds: 0, NoDataBehavior: "report", ErrorBehavior: "keep"}
}
func (f alertFixture) commit(pending db.PendingAction) (resource.ActionCommitResult, error) {
	var result resource.ActionCommitResult
	extension := ActionExtension{}
	workflow := actionservice.Service{Store: f.s.Store, Idempotency: f.s.Actions.Idempotency, Resources: resource.Service{Store: f.s.Store, Actions: f.s.Actions, Extension: extension}}
	confirmed, err := workflow.Confirm(f.t.Context(), pending.CreatorSubjectID.String(), uuid.NewString(), pending.EnterpriseID, pending.AuthorizationVersion, false, pending.ActionRef, uuid.NewString())
	if err != nil {
		return result, err
	}
	err = alertTransaction(f.t.Context(), f.s.Store, func(q *db.Queries) error {
		var e error
		result, e = f.s.Actions.ExecuteReady(f.t.Context(), q, confirmed.PendingAction, extension.RevalidateAction, extension.CommitAction)
		return e
	})
	return result, err
}
func (f alertFixture) createPolicy() db.AlertConfiguration {
	f.t.Helper()
	raw, _ := json.Marshal(f.policy())
	p, e := f.s.Preview(f.t.Context(), f.a, Mutation{Kind: "policy", Operation: "create", Name: "Pressure", Spec: raw}, uuid.NewString())
	if e != nil {
		f.t.Fatal(e)
	}
	result, e := f.commit(p)
	if e != nil {
		f.t.Fatal(e)
	}
	row, e := f.s.Store.Queries.GetAlertConfiguration(f.t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: result.ResourceID})
	if e != nil {
		f.t.Fatal(e)
	}
	return row
}
func (f alertFixture) task(queue string) runtime.Task {
	id := uuid.New()
	f.exec("INSERT INTO runtime_tasks(id,enterprise_id,queue,status,lease_owner,lease_until,fence_token) VALUES($1,$2,$3,'running','test-worker',now()+interval '1 hour',1)", id, f.a.EnterpriseID, queue)
	return runtime.Task{RuntimeTask: db.RuntimeTask{ID: id, EnterpriseID: nullID(f.a.EnterpriseID), Queue: queue, LeaseOwner: pgtype.Text{String: "test-worker", Valid: true}, FenceToken: 1}}
}
func TestPostgresAlertPreviewAtomicScopeAndConfirmation(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	p := f.policy()
	raw, _ := json.Marshal(p)
	key := uuid.NewString()
	preview, e := f.s.Preview(ctx, f.a, Mutation{Kind: "policy", Operation: "create", Name: "Pressure", Spec: raw}, key)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := f.s.Configurations(ctx, f.a, "policy")
	if e != nil || len(rows) != 0 {
		t.Fatal("preview changed configuration", e)
	}
	replay, e := f.s.Preview(ctx, f.a, Mutation{Kind: "policy", Operation: "create", Name: "Pressure", Spec: raw}, key)
	if e != nil || replay.ID != preview.ID {
		t.Fatal("preview idempotency", e)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	for range 2 {
		go func() { defer wg.Done(); _, e := f.commit(preview); results <- e }()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e != nil {
			t.Log("confirmation:", e)
		}
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent confirmations succeeded %d times", success)
	}
	rows, e = f.s.Configurations(ctx, f.a, "policy")
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	spec, _ := decodeSpec[PolicySpec](rows[0].Spec)
	if len(spec.Rules) != 2 {
		t.Fatal("rules not atomically committed")
	}
	other := f.a
	other.EnterpriseID = uuid.New()
	if _, e = f.s.Configuration(ctx, other, rows[0].ID); e == nil {
		t.Fatal("cross-enterprise read")
	}
	next := uuid.New()
	f.addHost(next, "web-prod-02")
	eval, e := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	if e != nil {
		t.Fatal(e)
	}
	ids, e := selectedResources(ctx, f.s.Store.Queries, eval, spec.Resources)
	if e != nil || len(ids) != 1 {
		t.Fatal(ids, e)
	}
	f.grant("service_account", f.sa, next)
	ids, e = selectedResources(ctx, f.s.Store.Queries, eval, spec.Resources)
	if e != nil || len(ids) != 2 {
		t.Fatal("new authorized resource not included", ids, e)
	}
}
func TestPostgresAlertLifecycleFenceNoDataHistoryAndRevocation(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	row := f.createPolicy()
	p, _ := decodeSpec[PolicySpec](row.Spec)
	task := f.task(EvaluationQueue)
	eval, _ := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	at := time.Now().UTC().Truncate(30 * time.Second)
	obs := Observation{Key: "cpu:" + f.host.String(), Rule: p.Rules[0], RuleID: p.Rules[0].ID, Value: 95, ResourceIDs: []uuid.UUID{f.host}, SourceIDs: []uuid.UUID{}, Labels: map[string]string{"argus_resource_id": f.host.String()}, Firing: true, Severity: "critical"}
	worker := Worker{Service: f.s}
	result := Evaluation{Observations: []Observation{obs}, ResourceIDs: []uuid.UUID{f.host}, Sources: []db.AlertMetricSourcesRow{}, Evaluator: eval, Slot: at, Health: "normal"}
	_, snapshot, scopeErr := metricSources(ctx, f.s.Store.Queries, eval, result.ResourceIDs, nil, p.Resources.Types)
	if scopeErr != nil {
		t.Fatal(scopeErr)
	}
	result.ResourceRefs = snapshot.Refs()
	result.ScopeSignature = snapshot.Signature
	stale := task
	stale.FenceToken = 0
	if e := worker.persistEvaluation(ctx, stale, row, p, result); !errors.Is(e, ErrConflict) {
		t.Fatal("stale fence admitted", e)
	}
	if e := worker.persistEvaluation(ctx, task, row, p, result); e != nil {
		t.Fatal(e)
	}
	page, e := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if e != nil || len(page.Items) != 1 || page.Items[0].Lifecycle != "firing" {
		t.Fatal(page, e)
	}
	id := page.Items[0].ID
	result.Slot = at.Add(30 * time.Second)
	result.Health = "error"
	result.ErrorCode = "ALERT_QUERY_FAILED"
	result.Observations = nil
	if e = worker.persistEvaluation(ctx, task, row, p, result); e != nil {
		t.Fatal(e)
	}
	value, events, e := f.s.Instance(ctx, f.a, id)
	if e != nil || value.Lifecycle != "firing" || value.Health != "error" || len(events) != 1 {
		t.Fatalf("query failure: lifecycle=%s health=%s events=%d err=%v", value.Lifecycle, value.Health, len(events), e)
	}
	result.Slot = at.Add(60 * time.Second)
	result.Health = "normal"
	obs.Firing = false
	obs.Recovering = true
	obs.Value = 70
	result.Observations = []Observation{obs}
	if e = worker.persistEvaluation(ctx, task, row, p, result); e != nil {
		t.Fatal(e)
	}
	value, events, e = f.s.Instance(ctx, f.a, id)
	if e != nil || value.Lifecycle != "resolved" || len(events) != 2 {
		t.Fatal(value, e)
	}
	page, e = f.s.Instances(ctx, f.a, ListFilter{}, true)
	if e != nil || len(page.Items) != 1 {
		t.Fatal("history lost", e)
	}
	f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_type='user'", f.a.EnterpriseID)
	if _, _, e = f.s.Instance(ctx, f.a, id); !errors.Is(e, ErrDenied) {
		t.Fatal("revoked instance readable", e)
	}
	f.exec("UPDATE service_accounts SET status='disabled' WHERE id=$1", f.sa)
	if _, e = evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID); !errors.Is(e, ErrEvaluator) {
		t.Fatal("disabled evaluator admitted", e)
	}
}

var _ = context.Background
