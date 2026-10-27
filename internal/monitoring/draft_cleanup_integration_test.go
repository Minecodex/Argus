package monitoring

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresMonitoringDraftCleanupPreservesBoundAndPlatformSecrets(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	input := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "mysql", TargetName: "mysql", Integration: "mysql", CollectorID: collector, TargetIdentity: json.RawMessage(`{"endpoint":"db:3306"}`), Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}
	material := json.RawMessage(`{"username":"monitor","password":"test-only-private-draft"}`)
	if _, err := f.applyCredentials(input, material); err != nil {
		t.Fatal(err)
	}
	var cancelled uuid.UUID
	for i := 0; i < 2; i++ {
		pending, err := f.s.PreviewWithCredentials(ctx, f.a, input, material, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			cancelled = pending.ID
			f.exec("UPDATE pending_actions SET status='cancelled' WHERE id=$1", pending.ID)
		}
	}
	f.exec("UPDATE monitoring_credential_drafts SET expires_at=now()-interval '1 second' WHERE enterprise_id=$1 AND id NOT IN (SELECT (immutable_plan->'mutation'->'collection'->>'credential_draft_id')::uuid FROM pending_action_plans WHERE pending_action_id=$2)", f.a.EnterpriseID, cancelled)
	platformID := uuid.New()
	f.exec("INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'Keep platform secret','api_token',$3)", platformID, f.a.EnterpriseID, f.a.SubjectID)
	count, err := f.s.Credentials.CleanupMonitoringDrafts(ctx, 500)
	if err != nil || count < 2 {
		t.Fatalf("draft cleanup: %d %v", count, err)
	}
	var drafts, bound, platform int
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_credential_drafts WHERE enterprise_id=$1 AND consumed_at IS NULL", f.a.EnterpriseID).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM secrets WHERE enterprise_id=$1 AND owner_type='monitoring_collection' AND status='active'", f.a.EnterpriseID).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM secrets WHERE enterprise_id=$1 AND id=$2 AND purpose='platform'", f.a.EnterpriseID, platformID).Scan(&platform); err != nil {
		t.Fatal(err)
	}
	if drafts != 0 || bound != 1 || platform != 1 {
		t.Fatalf("cleanup exceeded ownership: drafts=%d bound=%d platform=%d", drafts, bound, platform)
	}
}
