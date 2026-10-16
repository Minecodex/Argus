package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"testing"
	"time"
)

type alertQueryFixture struct {
	resource, source uuid.UUID
	missing, partial string
}

func (q alertQueryFixture) ExecuteEngineQuery(_ context.Context, r queryengine.Request) (queryengine.Result, error) {
	if !r.Start.Before(r.End) {
		return queryengine.Result{}, fmt.Errorf("invalid Query RPC time range")
	}
	values := []any{}
	if r.Expression != q.missing {
		values = append(values, map[string]any{"metric": map[string]string{"argus_resource_id": q.resource.String(), "argus_source_id": q.source.String(), "service": "api"}, "value": []any{r.Start.Unix(), "95"}})
	}
	return queryengine.Result{ResultType: "vector", Data: map[string]any{"resultType": "vector", "result": values}, Meta: queryengine.QueryMeta{Partial: r.Expression == q.partial}}, nil
}

func TestPostgresEvaluationIsolatesMissingRulesAndKeepsCompositeUnknown(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	policy := f.createPolicy()
	p, _ := decodeSpec[PolicySpec](policy.Spec)
	collector, source, generation := uuid.New(), uuid.New(), uuid.New()
	distribution := uuid.New()
	f.exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Plan6',$2,'fixture','v1','supported','[]',1)", distribution, distribution.String())
	f.exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,effective_revision,desired_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.a.EnterpriseID, f.host, distribution, generation)
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,ARRAY[$3]::uuid[],'{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector, uuid.New())
	f.exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,resource_type,resource_id,source_key,source_type,generation,config_revision,config_hash,signals) VALUES($1,$2,$3,'host',$4,$5,'host',$6,1,decode(repeat('00',32),'hex'),ARRAY['metrics'])", source, f.a.EnterpriseID, collector, f.host, source.String(), generation)
	eval, err := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Telemetry = telemetry.Service{Store: f.s.Store, Engine: alertQueryFixture{resource: f.host, source: source, missing: p.Rules[1].Expression}}
	result, err := f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, time.Now().UTC().Truncate(30*time.Second))
	if err != nil || result.Health != "no_data" || len(result.Observations) != 1 || len(result.Outcomes) != 2 {
		t.Fatal(result, err)
	}
	if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, result); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil {
		t.Fatal(err)
	}
	healthy := 0
	for _, i := range page.Items {
		if i.State.Kind == "condition" && i.Health == "normal" && i.Lifecycle == "firing" {
			healthy++
		}
	}
	if healthy != 1 {
		t.Fatal("missing rule blocked independent CPU", page)
	}
	p.Mode = "every"
	result, err = f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, time.Now().UTC())
	if err != nil || len(result.Observations) != 0 || result.Health != "no_data" {
		t.Fatal(result, err)
	}
	f.s.Telemetry.Engine = alertQueryFixture{resource: f.host, source: source, partial: p.Rules[1].Expression}
	p.Mode = "independent"
	result, err = f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, time.Now().UTC())
	if err != nil || result.Health != "error" || len(result.Observations) != 1 {
		t.Fatal(result, err)
	}
	// A second exporter has no samples. Both rules for the first exporter are
	// complete, so their AND group must still advance independently.
	otherSource := uuid.New()
	f.exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,resource_type,resource_id,source_key,source_type,generation,config_revision,config_hash,signals) VALUES($1,$2,$3,'host',$4,$5,'host',$6,1,decode(repeat('00',32),'hex'),ARRAY['metrics'])", otherSource, f.a.EnterpriseID, collector, f.host, otherSource.String(), generation)
	f.s.Telemetry.Engine = alertQueryFixture{resource: f.host, source: source}
	p.Mode = "every"
	raw, _ := json.Marshal(p)
	action, err := f.s.Preview(ctx, f.a, Mutation{Kind: "policy", Operation: "update", ID: policy.ID, Name: policy.Name, ExpectedVersion: policy.Version, Spec: raw}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.commit(action); err != nil {
		t.Fatal(err)
	}
	policy, err = f.s.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: policy.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err = f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, time.Now().UTC().Truncate(30*time.Second).Add(30*time.Second))
	if err != nil || result.Health != "no_data" || len(result.Observations) != 1 || len(result.Observations[0].Members) != 2 {
		t.Fatal(result, err)
	}
	if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, result); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	for _, instance := range page.Items {
		complete = complete || instance.Health == "normal" && instance.Lifecycle == "firing" && len(instance.State.Members) == 2
	}
	if !complete {
		t.Fatal("missing source blocked a complete AND group", page)
	}
	// A sub-microsecond remainder denotes the same persisted slot. It must
	// not replace the successful value/health with a duplicate failure.
	duplicate := Evaluation{Evaluator: eval, ResourceIDs: []uuid.UUID{f.host}, Sources: result.Sources, Health: "error", Slot: result.Slot.Add(100 * time.Nanosecond)}
	if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, duplicate); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range page.Items {
		if instance.State.Kind == "condition" && len(instance.State.Members) == 2 && (instance.Health != "normal" || instance.State.LastSuccessfulAt == nil || !instance.State.LastSuccessfulAt.Equal(result.Slot)) {
			t.Fatal("same persisted slot was applied twice", instance.Health, instance.State.LastSuccessfulAt)
		}
	}
	// Exercise the persisted failure path, including public DTO freshness. An
	// unsuccessful attempt advances health/time but retains the prior values.
	for index, health := range []string{"error", "no_data"} {
		failed := Evaluation{Evaluator: eval, ResourceIDs: []uuid.UUID{f.host}, Sources: result.Sources, Health: health, Slot: result.Slot.Add(time.Duration(index+1) * 30 * time.Second)}
		if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, failed); err != nil {
			t.Fatal(err)
		}
		page, err = f.s.Instances(ctx, f.a, ListFilter{}, false)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, instance := range page.Items {
			if instance.State.Kind != "condition" || len(instance.State.Members) != 2 {
				continue
			}
			found = true
			if instance.Health != health || instance.Lifecycle != "firing" || instance.State.LastSuccessfulAt == nil || !instance.State.LastSuccessfulAt.Equal(result.Slot) || !instance.LastEvaluatedAt.Equal(failed.Slot.Truncate(time.Microsecond)) {
				t.Fatal("failure changed successful value provenance", instance)
			}
		}
		if !found {
			t.Fatal("missing retained composite instance")
		}
	}
	result.Slot = result.Slot.Add(90 * time.Second)
	if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, result); err != nil {
		t.Fatal(err)
	}
	// Revocation does not provide authoritative evidence of a changed source.
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_type='service_account' AND subject_id=$2", f.a.EnterpriseID, f.sa)
	f.exec("UPDATE service_accounts SET authorization_version=authorization_version+1 WHERE id=$1", f.sa)
	failed := Evaluation{Evaluator: eval, ResourceIDs: []uuid.UUID{f.host}, Sources: []db.AlertMetricSourcesRow{}, Observations: []Observation{}, Health: "authorization_denied", ErrorCode: "ALERT_SCOPE_DENIED", Slot: result.Slot.Add(30 * time.Second)}
	if err = (Worker{Service: f.s}).persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, failed); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil {
		t.Fatal(err)
	}
	retained := false
	for _, instance := range page.Items {
		retained = retained || instance.Health == "authorization_denied" && instance.Lifecycle == "firing" && len(instance.State.Members) == 2 && instance.State.LastSuccessfulAt != nil && instance.State.LastSuccessfulAt.Equal(result.Slot) && instance.LastEvaluatedAt.Equal(failed.Slot.Truncate(time.Microsecond))
	}
	if !retained {
		t.Fatal("revocation was mistaken for source replacement", page)
	}
}

func TestDimensionSelectionCannotJoinDifferentResourceOrSource(t *testing.T) {
	a, b, sourceA, sourceB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	rules := []Rule{{ID: uuid.New(), Expression: "cpu", Operator: ">", Threshold: 85, RecoveryThreshold: 75, Severity: "critical"}, {ID: uuid.New(), Expression: "memory", Operator: ">", Threshold: 80, RecoveryThreshold: 70, Severity: "warning"}}
	sample := func(resource, source uuid.UUID, rule Rule) []Observation {
		raw := json.RawMessage(fmt.Sprintf(`{"resultType":"vector","result":[{"metric":{"argus_resource_id":"%s","argus_source_id":"%s","service":"same"},"value":[1,"95"]}]}`, resource, source))
		var data any
		_ = json.Unmarshal(raw, &data)
		obs, err := decodeObservations(queryengine.Result{Data: data, ResultType: "vector"}, rule, []string{"service"}, []uuid.UUID{a, b})
		if err != nil {
			t.Fatal(err)
		}
		return obs
	}
	p := PolicySpec{Mode: "every", Rules: rules}
	for _, pair := range [][2]uuid.UUID{{b, sourceA}, {a, sourceB}} {
		if out := combineObservations(p, [][]Observation{sample(a, sourceA, rules[0]), sample(pair[0], pair[1], rules[1])}); len(out) != 0 {
			t.Fatal("identity removed by custom dimensions", out)
		}
	}
}
