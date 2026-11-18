package alerting

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresLegacyProjectedPendingStateCannotCorruptSharedPolicy(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	other := uuid.New()
	f.addHost(other, "legacy-snapshot-second-host")
	f.grant("user", f.a.SubjectID, other)
	f.grant("service_account", f.sa, other)
	p := f.policy()
	p.Resources = ResourceSelector{Mode: "explicit", Types: []string{"host"}, IDs: []uuid.UUID{f.host, other}}
	p.Rules = p.Rules[:1]
	p.ForSeconds = 30
	encoded, _ := json.Marshal(p)
	preview, err := f.s.Preview(ctx, f.a, Mutation{Kind: "policy", Operation: "create", Name: "Legacy pending snapshot", Spec: encoded}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.commit(preview)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := f.s.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = decodeSpec[PolicySpec](policy.Spec)
	original := append([]byte(nil), policy.Spec...)
	actor, err := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := selectedResources(ctx, f.s.Store.Queries, actor, p.Resources)
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := metricSources(ctx, f.s.Store.Queries, actor, ids, nil, p.Resources.Types)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(10 * time.Second)
	observation := Observation{Key: "legacy:" + f.host.String(), Rule: p.Rules[0], RuleID: p.Rules[0].ID, Value: 95, ResourceIDs: []uuid.UUID{f.host}, SourceIDs: []uuid.UUID{}, Labels: map[string]string{"argus_resource_id": f.host.String()}, Firing: true, Severity: "critical"}
	result := Evaluation{Observations: []Observation{observation}, ResourceIDs: ids, Sources: []db.AlertMetricSourcesRow{}, Evaluator: actor, Slot: at, Health: "normal", ResourceRefs: scope.Refs(), ScopeSignature: scope.Signature}
	w := Worker{Service: f.s}
	task := f.task(EvaluationQueue)
	if err = w.persistEvaluation(ctx, task, policy, p, result); err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.Store.Queries.ListPolicyAlertInstances(ctx, db.ListPolicyAlertInstancesParams{EnterpriseID: f.a.EnterpriseID, PolicyID: policy.ID})
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	// Reproduce the pre-fix noise writer's persisted public projection. This
	// fixture is only in the disposable test DB; production facts are not edited.
	public, err := instanceDTO(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(public.State)
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE alert_instances SET state=$2 WHERE id=$1", rows[0].ID, legacy)
	for step := 1; step <= 3; step++ {
		result.Slot = at.Add(time.Duration(step) * 10 * time.Second)
		if err = w.persistEvaluation(ctx, task, policy, p, result); err != nil {
			t.Fatalf("slot %d: %v", step, err)
		}
		if !bytes.Equal(policy.Spec, original) || !json.Valid(policy.Spec) {
			t.Fatal("stored-state decode mutated the shared policy definition")
		}
	}
	rows, err = f.s.Store.Queries.ListPolicyAlertInstances(ctx, db.ListPolicyAlertInstancesParams{EnterpriseID: f.a.EnterpriseID, PolicyID: policy.ID})
	if err != nil || len(rows) != 1 || rows[0].Lifecycle != "firing" {
		t.Fatal("continuous pending window did not advance", rows, err)
	}
}
