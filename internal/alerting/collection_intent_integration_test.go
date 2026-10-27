package alerting

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func TestPostgresExplicitStopSuspendsWithoutNoDataOrRecovery(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	policy := f.createPolicy()
	p, _ := decodeSpec[PolicySpec](policy.Spec)
	collector, source, generation, distribution, collection := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Stop test',$2,'fixture','v1','supported','[]',1)", distribution, distribution.String())
	f.exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,effective_revision,desired_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.a.EnterpriseID, f.host, distribution, generation)
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,ARRAY[$3]::uuid[],'{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector, uuid.New())
	f.exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,resource_type,resource_id,source_key,source_type,generation,config_revision,config_hash,signals) VALUES($1,$2,$3,'host',$4,$5,'host',$6,1,decode(repeat('00',32),'hex'),ARRAY['metrics'])", source, f.a.EnterpriseID, collector, f.host, source.String(), generation)
	f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,'Host metrics','host_metrics','host',$3,$4,$5)", collection, f.a.EnterpriseID, f.host, collector, f.a.SubjectID)
	eval, err := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Telemetry = telemetry.Service{Store: f.s.Store, Engine: alertQueryFixture{resource: f.host, source: source}}
	w := Worker{Service: f.s}
	at := time.Now().UTC().Truncate(time.Second)
	apply := func(want string) {
		t.Helper()
		r, e := f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, at)
		if e != nil || r.Health != want {
			t.Fatalf("health %q, want %q: %v", r.Health, want, e)
		}
		if e = w.persistEvaluation(ctx, f.task(EvaluationQueue), policy, p, r); e != nil {
			t.Fatal(e)
		}
		at = at.Add(30 * time.Second)
	}
	apply("normal")
	before, err := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil || len(before.Items) != 2 {
		t.Fatal(before, err)
	}
	f.exec("UPDATE monitoring_collections SET desired_enabled=false WHERE id=$1", collection)
	apply("not_evaluated")
	after, err := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil || len(after.Items) != len(before.Items) {
		t.Fatal("stop created an extra health episode", after, err)
	}
	for _, item := range after.Items {
		if item.Health != "not_evaluated" || item.Lifecycle != "firing" || item.State.PendingSince != nil || item.State.RecoveringSince != nil {
			t.Fatal("stop fabricated recovery or retained duration streak", item)
		}
	}
	f.exec("UPDATE monitoring_collections SET desired_enabled=true WHERE id=$1", collection)
	apply("normal")
	// An enabled receiver with absent observations still reports a real data gap.
	f.s.Telemetry.Engine = alertQueryFixture{resource: f.host, source: source, missing: p.Rules[0].Expression}
	r, err := f.s.evaluate(ctx, eval, p, []uuid.UUID{f.host}, at)
	if err != nil || r.Health != "no_data" {
		t.Fatal("unexpected missing data was suspended", r.Health, err)
	}
	// A second resource without an explicit stop must prevent global suspension.
	other := uuid.New()
	f.addHost(other, "other")
	f.grant("service_account", eval.SubjectID, other)
	_, snapshot, err := metricSources(ctx, f.s.Store.Queries, eval, []uuid.UUID{f.host, other}, nil, p.Resources.Types)
	if err != nil {
		t.Fatal(err)
	}
	if stopped, e := metricScopeExplicitlyStopped(ctx, f.s.Store.Queries, eval, snapshot); e != nil || stopped {
		t.Fatal("one stopped target silenced another target", e)
	}

}

func TestPostgresMonitoringTargetStopDoesNotRequireAnOwnedCollector(t *testing.T) {
	f := newAlertFixture(t)
	stopped, peer, missing := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{stopped, peer, missing} {
		f.exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'Owned web target','web','nginx','{\"endpoint\":\"http://web/status\"}',$3)", id, f.a.EnterpriseID, f.a.SubjectID)
	}
	for _, id := range []uuid.UUID{stopped, peer} {
		f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,desired_enabled,created_by) VALUES($1,$2,'Owned web collection','nginx','monitoring_target',$3,$4,$5)", uuid.New(), f.a.EnterpriseID, id, id == peer, f.a.SubjectID)
	}
	check := func(want bool, ids ...uuid.UUID) {
		t.Helper()
		scope := observationscope.Snapshot{}
		for _, id := range ids {
			scope.Resources = append(scope.Resources, observationscope.Resource{Ref: observationscope.Ref{Kind: "monitoring_target", ID: id}})
		}
		got, err := metricScopeExplicitlyStopped(t.Context(), f.s.Store.Queries, f.a, scope)
		if err != nil || got != want {
			t.Fatalf("explicit target stop=%v want %v err=%v", got, want, err)
		}
	}
	check(true, stopped)
	check(false, peer)
	check(false, missing)
	check(false, stopped, peer)
	f.exec("UPDATE monitoring_collections SET desired_enabled=true WHERE target_id=$1", stopped)
	check(false, stopped)
}
