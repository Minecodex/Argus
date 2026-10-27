package alerting

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestPostgresHandlingConfirmationSurvivesSuccessfulEvaluation(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	policy := f.createPolicy()
	p, _ := decodeSpec[PolicySpec](policy.Spec)
	eval, _ := evaluator(ctx, f.s.Store.Queries, f.a.EnterpriseID)
	task := f.task(EvaluationQueue)
	obs := Observation{Key: "cpu:" + f.host.String(), Rule: p.Rules[0], RuleID: p.Rules[0].ID, Value: 95, ResourceIDs: []uuid.UUID{f.host}, SourceIDs: []uuid.UUID{}, Labels: map[string]string{"argus_resource_id": f.host.String()}, Firing: true, Severity: "critical"}
	result := Evaluation{Observations: []Observation{obs}, ResourceIDs: []uuid.UUID{f.host}, Sources: []db.AlertMetricSourcesRow{}, Evaluator: eval, Slot: time.Now().UTC().Truncate(30 * time.Second), Health: "normal"}
	_, snapshot, scopeErr := metricSources(ctx, f.s.Store.Queries, eval, result.ResourceIDs, nil, p.Resources.Types)
	if scopeErr != nil {
		t.Fatal(scopeErr)
	}
	result.ResourceRefs = snapshot.Refs()
	result.ScopeSignature = snapshot.Signature
	w := Worker{Service: f.s}
	if err := w.persistEvaluation(ctx, task, policy, p, result); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	first := page.Items[0]
	preview, err := f.s.PreviewCommand(ctx, f.a, Command{Operation: "event.acknowledge", ID: first.ID, ExpectedHandlingVersion: first.HandlingVersion}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	result.Slot = result.Slot.Add(30 * time.Second)
	if err = w.persistEvaluation(ctx, task, policy, p, result); err != nil {
		t.Fatal(err)
	}
	if _, err = f.commit(preview); err != nil {
		t.Fatal("evaluation invalidated handling confirmation", err)
	}
	current, events, err := f.s.Instance(ctx, f.a, first.ID)
	if err != nil || !current.Acknowledged || current.HandlingVersion != 2 || len(events) != 2 {
		t.Fatal(current, events, err)
	}
	if _, err = f.s.Handle(ctx, f.a, first.ID, HandleInput{Operation: "comment", Comment: "concurrent stale handling", ExpectedHandlingVersion: 1}, uuid.NewString(), "admin_ui"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale handling admitted", err)
	}
	if _, err = f.s.Handle(ctx, f.a, first.ID, HandleInput{Operation: "comment", Comment: "verified", ExpectedHandlingVersion: 2}, uuid.NewString(), "admin_ui"); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoppedEvaluatorRemainsManageableWithoutCreatorBypass(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	policy := f.createPolicy()
	f.exec("UPDATE service_accounts SET status='disabled' WHERE id=$1", f.sa)
	if _, err := f.s.Configuration(ctx, f.a, policy.ID); err != nil {
		t.Fatal("authorized policy vanished", err)
	}
	pending, err := f.s.Preview(ctx, f.a, Mutation{Kind: "policy", Operation: "disable", ID: policy.ID, ExpectedVersion: policy.Version}, uuid.NewString())
	if err != nil {
		t.Fatal("cannot stop policy while evaluator is stopped", err)
	}
	if _, err = f.commit(pending); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_type='user'", f.a.EnterpriseID)
	if _, err = f.s.Configuration(ctx, f.a, policy.ID); !errors.Is(err, ErrDenied) {
		t.Fatal("creator bypassed resource revocation", err)
	}
}

func TestPostgresSettingsPreviewAtomicAndRequiresServiceAccountPermission(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	rows, err := f.s.Configurations(ctx, f.a, "settings")
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	raw, _ := json.Marshal(SettingsSpec{EvaluatorID: f.sa})
	preview, err := f.s.Preview(ctx, f.a, Mutation{Kind: "settings", Operation: "update", ID: rows[0].ID, Name: "Selected evaluator", Spec: raw, ExpectedVersion: rows[0].Version}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.commit(preview); err != nil {
		t.Fatal(err)
	}
	f.exec("DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM role_bindings WHERE subject_id=$1) AND permission_id='service_account.manage'", f.a.SubjectID)
	rows, _ = f.s.Configurations(ctx, f.a, "settings")
	_, err = f.s.Preview(ctx, f.a, Mutation{Kind: "settings", Operation: "update", ID: rows[0].ID, Name: "Denied", Spec: raw, ExpectedVersion: rows[0].Version}, uuid.NewString())
	if !errors.Is(err, ErrDenied) {
		t.Fatal("settings mutation without IAM permission admitted", err)
	}
}
