package alerting

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestInstanceSnapshotDoesNotDiscloseSiblingPolicyResources(t *testing.T) {
	a, b, sourceA, sourceB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	policy := PolicySpec{Resources: ResourceSelector{Mode: "dynamic", Types: []string{"host"}, IDs: []uuid.UUID{a, b}, SourceIDs: []uuid.UUID{sourceA, sourceB}, Matchers: []Matcher{{Key: "private-team", Operator: "=", Value: "other-team"}}}, Rules: []Rule{{ID: uuid.New(), Expression: "cpu_usage", Threshold: 85}}}
	p, _ := json.Marshal(policy)
	sources, _ := json.Marshal([]db.AlertMetricSourcesRow{{ID: sourceA, ResourceID: a}, {ID: sourceB, ResourceID: b}})
	state, _ := json.Marshal(InstanceState{PolicySnapshot: p, SourceSnapshot: sources, LatestSourceSnapshot: sources})
	original := append([]byte(nil), state...)
	row := db.AlertInstance{ResourceIds: []uuid.UUID{a}, SourceIds: []uuid.UUID{sourceA}, State: state}
	instance, err := instanceDTO(row)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(instance)
	for _, restricted := range []string{b.String(), sourceB.String(), "other-team"} {
		if bytes.Contains(public, []byte(restricted)) {
			t.Fatalf("instance exposed unauthorized sibling: %s", restricted)
		}
	}
	if !bytes.Contains(public, []byte(a.String())) || !bytes.Contains(public, []byte(sourceA.String())) {
		t.Fatal("actual participants missing")
	}
	if !bytes.Equal(row.State, original) {
		t.Fatal("public projection changed immutable stored facts")
	}
}
