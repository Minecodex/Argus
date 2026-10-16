package alerting

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresAlertAuditPersistsScopeAndConfirmationExecutionLinks(t *testing.T) {
	f := newAlertFixture(t)
	raw, _ := json.Marshal(f.policy())
	pending, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: "policy", Operation: "create", Name: "Audit links", Spec: raw}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.commit(pending); err != nil {
		t.Fatal(err)
	}
	var preview, confirmation, commit int
	err = f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE action='alert.policy.create.preview'),count(*) FILTER(WHERE action='pending_action.confirm' AND details->>'execution_id' IS NOT NULL),count(*) FILTER(WHERE action='alert.policy.create' AND details->>'execution_id' IS NOT NULL) FROM audit_events WHERE enterprise_id=$1 AND details->>'pending_action_id'=$2 AND details->'resource_ids' ? $3`, f.a.EnterpriseID, pending.ID.String(), f.host.String()).Scan(&preview, &confirmation, &commit)
	if err != nil || preview != 1 || confirmation != 1 || commit != 1 {
		t.Fatal("audit dropped authorized scope or action/execution links", preview, confirmation, commit, err)
	}
}
