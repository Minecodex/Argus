package monitoring

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresCollectionStatesExposeDistinctCollectorAnchorsWithoutHostFacts(t *testing.T) {
	f := newFixture(t)
	configured := f.collector()
	f.exec("UPDATE hosts SET name='identity-anchor-a' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", configured)
	effective := f.collector()
	created, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{
		Name: "identity-anchors", TargetName: "private-placement", Integration: "nginx",
		TargetIdentity: json.RawMessage(`{"endpoint":"http://10.0.0.8/status"}`), CollectorID: configured,
		Config: json.RawMessage(`{}`), Enabled: false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(t.Context(), db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	ref := ResourceRef{Kind: "monitoring_target", ID: row.TargetID}
	query := db.ObservationCollectionRevisionParams{EnterpriseID: f.a.EnterpriseID, TargetType: ref.Kind, TargetID: ref.ID}
	before, err := f.s.Store.Queries.ObservationCollectionRevision(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	states, more, err := f.s.CollectionStates(t.Context(), f.a, ref, uuid.Nil, 25)
	if err != nil || more || len(states) != 1 || states[0].ConfiguredCollectorID == nil || *states[0].ConfiguredCollectorID != configured || states[0].EffectiveCollectorID != nil || states[0].PlacementState != "requires_authorized_resolution" {
		t.Fatal("configured identity was lost or effective placement was guessed", states, err)
	}
	// A target-only reader may read the same Collector anchor already present in
	// collection.get, but this must not reveal any Host identity or environment.
	grant := uuid.New()
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", grant, f.a.EnterpriseID, f.other.SubjectID, ref.ID)
	states, _, err = f.s.CollectionStates(t.Context(), f.other, ref, uuid.Nil, 25)
	encoded, _ := json.Marshal(states)
	if err != nil || len(states) != 1 || !strings.Contains(string(encoded), configured.String()) {
		t.Fatal("target reader lost its authorized collection identity", err)
	}
	for _, forbidden := range []string{"identity-anchor-a", "development", "10.0.0.8", "resource_id", "host_id", "credentials", "config\""} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("collection identity expanded into unauthorized placement/configuration", forbidden)
		}
	}
	f.exec("UPDATE monitoring_collections SET effective_collector_id=$2,effective_assignment_epoch=1 WHERE id=$1", row.ID, effective)
	after, err := f.s.Store.Queries.ObservationCollectionRevision(t.Context(), query)
	if err != nil || before == after {
		t.Fatal("effective Collector changed without invalidating continuation identity", err)
	}
	states, _, err = f.s.CollectionStates(t.Context(), f.other, ref, uuid.Nil, 25)
	if err != nil || len(states) != 1 || states[0].ConfiguredCollectorID == nil || *states[0].ConfiguredCollectorID != configured || states[0].EffectiveCollectorID == nil || *states[0].EffectiveCollectorID != effective {
		t.Fatal("configured and effective identities were merged during migration", err)
	}
	f.exec("UPDATE monitoring_collections SET assignment_epoch=assignment_epoch+1 WHERE id=$1", row.ID)
	changedEpoch, err := f.s.Store.Queries.ObservationCollectionRevision(t.Context(), query)
	if err != nil || changedEpoch == after {
		t.Fatal("assignment epoch reused a stale continuation identity", err)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	if _, _, err := f.s.CollectionStates(t.Context(), f.other, ref, uuid.Nil, 25); err == nil {
		t.Fatal("revoked target identity remained readable")
	}
}
