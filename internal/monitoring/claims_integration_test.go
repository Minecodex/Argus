package monitoring

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/collectionclaim"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresInstanceClaimMigrationRollbackAndPrimaryOwnership(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	a := f.collector()
	f.exec("UPDATE hosts SET name='Claim original host' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", a)
	b := f.collector()
	created, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "Claim database", TargetName: "Claim database", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"mysql.claim.test:3306"}`), CollectorID: a, Enabled: true, Config: json.RawMessage(`{}`), CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"monitor","password":"owned-test-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	spec := configbundle.Collection{ID: row.ID.String(), Integration: "mysql", TargetType: "monitoring_target", TargetID: row.TargetID.String(), Enabled: true, Revision: 1, AssignmentEpoch: 1, Config: json.RawMessage(`{}`)}
	rebuild := func(collector uuid.UUID, item configbundle.Collection) error {
		return f.s.Store.InTx(ctx, func(q *db.Queries) error {
			instance, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
			if err != nil {
				return err
			}
			return collectionclaim.Rebuild(ctx, q, f.a.EnterpriseID, instance, nil, []configbundle.Collection{item})
		})
	}
	if err := rebuild(a, spec); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(`{}`))
	primary := func() db.CollectionClaim {
		t.Helper()
		value, err := f.s.Store.Queries.GetActivePrimaryCollectionClaim(ctx, db.GetActivePrimaryCollectionClaimParams{EnterpriseID: f.a.EnterpriseID, PhysicalResourceRef: "monitoring_target:" + row.TargetID.String(), ClaimType: "mysql.metrics", SelectorHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if claim := primary(); claim.CollectorID != a || claim.CollectionID.UUID != row.ID || claim.AssignmentEpoch.Int64 != 1 {
		t.Fatal("instance claim did not own the stable database", claim)
	}
	spec.AssignmentEpoch = 2
	if err := rebuild(b, spec); err != nil {
		t.Fatal(err)
	}
	if primary().CollectorID != a {
		t.Fatal("candidate became primary before outcome")
	}
	if _, err := f.s.Store.Queries.RollbackCollectorClaimMigrations(ctx, db.RollbackCollectorClaimMigrationsParams{EnterpriseID: f.a.EnterpriseID, CollectorID: b}); err != nil {
		t.Fatal(err)
	}
	if primary().CollectorID != a {
		t.Fatal("failure lost the verified primary")
	}
	if err := rebuild(b, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Store.Queries.FinalizeCollectorClaimMigrations(ctx, db.FinalizeCollectorClaimMigrationsParams{EnterpriseID: f.a.EnterpriseID, CollectorID: b}); err != nil {
		t.Fatal(err)
	}
	if primary().CollectorID != b {
		t.Fatal("successful migration did not replace the primary")
	}
	conflict := spec
	conflict.ID = uuid.NewString()
	if err := rebuild(a, conflict); !errors.Is(err, collectionclaim.ErrConflict) {
		t.Fatal("another logical instance silently took over the target", err)
	}
}
