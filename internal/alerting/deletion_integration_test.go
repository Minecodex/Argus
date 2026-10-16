package alerting

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresScopedDeletePreviewSurvivesPlanSerialization(t *testing.T) {
	for _, kind := range []string{"policy", "silence", "inhibition"} {
		for _, input := range []string{"absent", "null"} {
			t.Run(kind+"/"+input, func(t *testing.T) {
				checkScopedDeletion(t, kind, input)
			})
		}
	}
}

func checkScopedDeletion(t *testing.T, kind, input string) {
	t.Helper()
	f := newAlertFixture(t)
	var value any = f.policy()
	if kind == "silence" {
		value = SilenceSpec{Enabled: true, ResourceIDs: []uuid.UUID{f.host}, StartsAt: time.Now().UTC(), EndsAt: time.Now().UTC().Add(time.Hour), Reason: "deployment"}
	} else if kind == "inhibition" {
		value = InhibitionSpec{Enabled: true, ResourceIDs: []uuid.UUID{f.host}, Source: []Matcher{{Key: "severity", Operator: "=", Value: "critical"}}, Target: []Matcher{{Key: "severity", Operator: "=", Value: "warning"}}, Equal: []string{"resource_id"}, Reason: "dependency"}
	}
	spec, _ := json.Marshal(value)
	preview, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: kind, Operation: "create", Name: "Maintenance", Spec: spec}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.commit(preview)
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: result.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	mutation := Mutation{Kind: kind, Operation: "delete", ID: row.ID, ExpectedVersion: row.Version}
	if input == "null" {
		mutation.Spec = json.RawMessage("null")
	}
	deletion, err := f.s.Preview(t.Context(), f.a, mutation, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var impact struct {
		ResourceIDs []uuid.UUID `json:"resource_ids"`
	}
	if json.Unmarshal(deletion.Preview, &impact) != nil || len(impact.ResourceIDs) != 1 || impact.ResourceIDs[0] != f.host {
		t.Fatalf("deletion lost the authorized resource scope: %s", deletion.Preview)
	}
	planRow, err := f.s.Store.Queries.GetPendingActionPlan(t.Context(), db.GetPendingActionPlanParams{ActionRef: deletion.ActionRef, EnterpriseID: deletion.EnterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	var plan ActionPlan
	if json.Unmarshal(planRow.ImmutablePlan, &plan) != nil {
		t.Fatal("invalid serialized deletion")
	}
	var hash []byte
	err = f.s.Store.InTx(t.Context(), func(q *db.Queries) error {
		var e error
		hash, e = (ActionExtension{}).RevalidateAction(t.Context(), q, deletion, planRow.ImmutablePlan)
		return e
	})
	if err != nil || !bytes.Equal(hash, deletion.ImpactHash) {
		t.Fatalf("serialized deletion changed impact hash: %v", err)
	}
	if _, err = f.commit(deletion); err != nil {
		t.Fatalf("serialized deletion changed its frozen impact: %v", err)
	}
	var status string
	var version int64
	err = f.s.Store.Pool.QueryRow(t.Context(), "SELECT status,version FROM alert_configurations WHERE enterprise_id=$1 AND id=$2", f.a.EnterpriseID, row.ID).Scan(&status, &version)
	if err != nil || status != "deleted" || version != 2 {
		t.Fatalf("deletion did not commit once: %s %d %v", status, version, err)
	}
	if _, err = f.commit(deletion); err == nil {
		t.Fatal("the same deletion committed twice")
	}
	var previews, confirms, commits int
	err = f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE action=$3 || '.preview'),count(*) FILTER(WHERE action='pending_action.confirm' AND details->>'execution_id' IS NOT NULL),count(*) FILTER(WHERE action=$3 AND details->>'execution_id' IS NOT NULL) FROM audit_events WHERE enterprise_id=$1 AND details->>'pending_action_id'=$2 AND details->'resource_ids' ? $4`, f.a.EnterpriseID, deletion.ID.String(), "alert."+kind+".delete", f.host.String()).Scan(&previews, &confirms, &commits)
	if err != nil || previews != 1 || confirms != 1 || commits != 1 {
		t.Fatalf("deletion scope/execution audit missing or duplicated: %d %d %d %v", previews, confirms, commits, err)
	}
}
