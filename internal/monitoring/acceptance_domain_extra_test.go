package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

var errAcceptanceAfterDomainWrites = errors.New("owned acceptance failure after domain writes")

type rollbackAcceptanceRuntime struct {
	transactionOnlyRuntime
	collector  uuid.UUID
	enterprise uuid.UUID
	observed   bool
}

func (r *rollbackAcceptanceRuntime) Commit(ctx context.Context, q *db.Queries, _ db.PendingAction, _ json.RawMessage) (resource.ActionCommitResult, error) {
	rows, err := q.ListCollectorMonitoringCollections(ctx, db.ListCollectorMonitoringCollectionsParams{EnterpriseID: r.enterprise, CollectorID: nullID(r.collector)})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if len(rows) != 1 || !rows[0].CredentialID.Valid {
		return resource.ActionCommitResult{}, errors.New("failure injection did not reach bound collection")
	}
	r.observed = true
	return resource.ActionCommitResult{}, errAcceptanceAfterDomainWrites
}

func TestPostgresAcceptanceCreateFailureRollsBackTargetCredentialAssignmentAndOutbox(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	runtime := &rollbackAcceptanceRuntime{collector: collector}
	runtime.enterprise = f.a.EnterpriseID
	f.s.Runtime = runtime
	input := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "atomic monitor", TargetName: "atomic target", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"owned-atomic:3306"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}
	_, err := f.applyCredentials(input, json.RawMessage(`{"username":"monitor","password":"owned-test-credential"}`))
	if !errors.Is(err, errAcceptanceAfterDomainWrites) || !runtime.observed {
		t.Fatal("expected post-write failure was not reached", err)
	}
	for _, table := range []string{"monitoring_targets", "monitoring_collections", "monitoring_collection_assignments", "monitoring_collection_revisions", "credentials"} {
		var count int
		if err := f.s.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed transaction left active %s: count=%d error=%v", table, count, err)
		}
	}
	var count int
	if err := f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE topic='monitoring.collection.changed' AND payload->>'enterprise_id'=$1`, f.a.EnterpriseID.String()).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed transaction left reconciliation work", count, err)
	}
	if err := f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FROM monitoring_credential_drafts WHERE enterprise_id=$1 AND consumed_at IS NOT NULL`, f.a.EnterpriseID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed transaction consumed private draft", count, err)
	}
	f.s.Runtime = transactionOnlyRuntime{}
	created, err := f.applyCredentials(input, json.RawMessage(`{"username":"monitor","password":"owned-recovered-credential"}`))
	if err != nil {
		t.Fatal("recovered transaction did not commit", err)
	}
	row, err := f.s.Collection(t.Context(), f.a, created.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	reused := input
	copy := *input.Collection
	copy.Name = "same target independent collection"
	copy.Target = row.Target
	copy.TargetIdentity = nil
	copy.TargetName = ""
	reused.Collection = &copy
	if _, err := f.applyCredentials(reused, json.RawMessage(`{"username":"monitor","password":"owned-independent-credential"}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("same target accepted duplicate active collection", err)
	}
	if _, err := f.apply(Mutation{Kind: "collection", Operation: "archive", ID: row.ID, ExpectedVersion: row.Version, Collection: &CollectionInput{}}); err != nil {
		t.Fatal("owned collection could not retire before stable-target reuse", err)
	}
	second, err := f.applyCredentials(reused, json.RawMessage(`{"username":"monitor","password":"owned-independent-credential"}`))
	if err != nil || second.ResourceID == created.ResourceID {
		t.Fatal("target reuse did not create independent collection", err)
	}
	if err := f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FROM monitoring_targets WHERE enterprise_id=$1`, f.a.EnterpriseID).Scan(&count); err != nil || count != 1 {
		t.Fatal("target reuse duplicated the stable target", count, err)
	}
	if err := f.s.Store.Pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE topic='monitoring.collection.changed' AND payload->>'enterprise_id'=$1`, f.a.EnterpriseID.String()).Scan(&count); err != nil || count != 3 {
		t.Fatal("successful transactions did not enqueue exactly once", count, err)
	}
}

func TestPostgresAcceptanceSameNamedObjectsRemainIsolatedAcrossEnterprises(t *testing.T) {
	first, second := newFixture(t), newFixture(t)
	type owned struct{ app, collector, collection, target uuid.UUID }
	rows := []owned{}
	for _, f := range []*fixture{first, second} {
		app, err := f.apply(Mutation{Kind: "application", Operation: "create", Application: &ApplicationInput{Key: "same-stable-key", Name: "Same enterprise-local name", Environment: "production"}})
		if err != nil {
			t.Fatal(err)
		}
		collector := f.collector()
		collection, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "same-monitor", TargetName: "same-target", Integration: "nginx", TargetIdentity: json.RawMessage(`{"endpoint":"http://same-target/status"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true}})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := f.s.Collection(t.Context(), f.a, collection.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, owned{app.ResourceID, collector, actual.ID, actual.Target.ID})
	}
	if rows[0].app == rows[1].app || rows[0].target == rows[1].target || rows[0].collector == rows[1].collector {
		t.Fatal("same names merged enterprise identities")
	}
	for i, f := range []*fixture{first, second} {
		foreign := rows[1-i]
		if _, err := f.s.Application(t.Context(), f.a, foreign.app); err == nil {
			t.Fatal("foreign application readable")
		}
		if _, err := f.s.Collection(t.Context(), f.a, foreign.collection); err == nil {
			t.Fatal("foreign collection readable")
		}
		page, err := f.s.Targets(t.Context(), f.a, ListInput{})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != rows[i].target {
			t.Fatal("foreign target leaked through catalog", err)
		}
		if _, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: "application", Operation: "archive", ID: foreign.app, ExpectedVersion: 1}, uuid.NewString()); err == nil {
			t.Fatal("foreign application writable")
		}
		if _, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "forged-executor", TargetName: "forged-target", Integration: "nginx", TargetIdentity: json.RawMessage(`{"endpoint":"http://same-target/status"}`), CollectorID: foreign.collector, Config: json.RawMessage(`{}`), Enabled: true}}, uuid.NewString()); err == nil {
			t.Fatal("foreign Collector accepted by same-name monitor")
		}
		forged := f.a
		if i == 0 {
			forged.EnterpriseID = second.a.EnterpriseID
		} else {
			forged.EnterpriseID = first.a.EnterpriseID
		}
		if _, err := f.s.Application(t.Context(), forged, foreign.app); err == nil {
			t.Fatal("enterprise parameter overrode subject ownership")
		}
	}
}
