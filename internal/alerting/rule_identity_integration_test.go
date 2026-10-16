package alerting

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresPolicyUpdateRequiresAndPreservesRuleIdentities(t *testing.T) {
	f := newAlertFixture(t)
	row := f.createPolicy()
	prior, err := decodeSpec[PolicySpec](row.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var countBefore, countAfter int
	count := func(target *int) {
		t.Helper()
		if err := f.s.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM pending_actions WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	count(&countBefore)
	mutation := Mutation{Kind: "policy", Operation: "update", ID: row.ID, Name: row.Name, ExpectedVersion: row.Version}
	missing := prior
	missing.Rules = append([]Rule(nil), prior.Rules...)
	missing.Rules[0].Threshold = 92
	missing.Rules[1].ID = uuid.Nil
	mutation.Spec, _ = json.Marshal(missing)
	if _, err = f.s.Preview(t.Context(), f.a, mutation, uuid.NewString()); !errors.Is(err, ErrInvalid) {
		t.Fatal("update without an unchanged rule identity accepted", err)
	}
	count(&countAfter)
	unchanged, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil || countBefore != countAfter || unchanged.Version != row.Version || !bytes.Equal(unchanged.Spec, row.Spec) {
		t.Fatal("rejected update created an action or changed configuration", err)
	}
	next := prior
	next.Rules = append([]Rule(nil), prior.Rules...)
	next.Rules[0].Threshold = 92
	mutation.Spec, _ = json.Marshal(next)
	preview, err := f.s.Preview(t.Context(), f.a, mutation, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var frozen struct {
		Spec PolicySpec `json:"spec"`
	}
	if err = json.Unmarshal(preview.Preview, &frozen); err != nil || frozen.Spec.Rules[1] != prior.Rules[1] {
		t.Fatal("preview rebuilt unchanged rule", err)
	}
	if _, err = f.commit(preview); err != nil {
		t.Fatal(err)
	}
	committed, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := decodeSpec[PolicySpec](committed.Spec)
	if err != nil || committed.Version != row.Version+1 || !reflect.DeepEqual(actual, next) {
		t.Fatal("narrow threshold commit changed another field")
	}
}
