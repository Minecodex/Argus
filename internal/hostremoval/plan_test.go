package hostremoval

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestRemovalPlanSurvivesCanonicalPersistence(t *testing.T) {
	dependency := Dependency{Type: "control_tunnel", ID: uuid.New(), Name: "198.51.100.28", Reason: "established"}
	current := Plan{SchemaVersion: "argus.host_removal/v1", HostID: uuid.New(), ConnectorID: uuid.New(), ExpectedVersion: 3,
		ConnectionEpoch: 4, ConnectorVersion: 2, Dependencies: []Dependency{dependency}, ComponentInventory: mustJSON([]Dependency{dependency}),
		ManagedChangeBefore:  json.RawMessage(`{"port":3389,"enabled":false}`),
		ManagedChangeApplied: json.RawMessage(`{"port":3389,"enabled":true}`), CreatedAt: time.Now().UTC()}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := resource.CanonicalJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var frozen Plan
	if err := json.Unmarshal(canonical, &frozen); err != nil {
		t.Fatal(err)
	}
	current.CreatedAt = current.CreatedAt.Add(time.Second)
	if !plansEqual(frozen, current) {
		t.Fatal("persisting the same removal authority changed its comparison")
	}
	for _, change := range []func(*Plan){
		func(p *Plan) { p.ExpectedVersion++ },
		func(p *Plan) { p.ConnectorID = uuid.New() },
		func(p *Plan) { p.ConnectionEpoch++ },
		func(p *Plan) { p.ComponentInventory = json.RawMessage(`[]`) },
		func(p *Plan) { p.ManagedChangeApplied = json.RawMessage(`{"enabled":false,"port":3389}`) },
	} {
		changed := current
		change(&changed)
		if plansEqual(frozen, changed) {
			t.Fatal("changed removal authority was accepted")
		}
	}
}

func TestRemovalPlanRejectsMalformedEmbeddedJSON(t *testing.T) {
	invalid := Plan{ComponentInventory: json.RawMessage(`{"broken":`)}
	if plansEqual(invalid, invalid) {
		t.Fatal("invalid authority was accepted")
	}
}

func removalPlanHashFixture(t *testing.T) (Plan, db.HostRemovalOperation) {
	t.Helper()
	plan := testPlan("windows_amd64")
	plan.ComponentInventory = mustJSON([]Dependency{{Type: "control_tunnel", ID: uuid.New(), Name: "target", Reason: "established"}})
	plan.ManagedChangeBefore = json.RawMessage(`{"port":3389,"enabled":false}`)
	plan.ManagedChangeApplied = json.RawMessage(`{"port":3389,"enabled":true}`)
	encoded, err := canonicalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(encoded)
	return plan, db.HostRemovalOperation{ID: plan.OperationID.UUID, HostID: plan.HostID, ConnectorID: plan.ConnectorID,
		RemovalGeneration: plan.RemovalGeneration, Plan: encoded, PlanHash: hash[:]}
}

func TestRemovalOperationPlanCanonicalRoundTrip(t *testing.T) {
	plan, operation := removalPlanHashFixture(t)
	// Struct encoding restores a different object-key order than canonical
	// storage, including embedded inventory and Windows restore objects.
	reordered, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	operation.Plan = reordered
	if _, err := DecodeOperationPlan(operation); err != nil {
		t.Fatal(err)
	}
	var mutated map[string]any
	if err := json.Unmarshal(operation.Plan, &mutated); err != nil {
		t.Fatal(err)
	}
	mutated["unexpected_authority"] = true
	operation.Plan, err = json.Marshal(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOperationPlan(operation); err == nil {
		t.Fatal("unhashed additional authority was accepted")
	}
}

func TestRemovalOperationPlanPostgresJSONB(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	connection, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(t.Context())
	_, operation := removalPlanHashFixture(t)
	var stored []byte
	if err := connection.QueryRow(t.Context(), "SELECT $1::jsonb", string(operation.Plan)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(operation.Plan, stored) {
		t.Fatal("fixture did not exercise JSONB normalization")
	}
	operation.Plan = stored
	if _, err := DecodeOperationPlan(operation); err != nil {
		t.Fatalf("persisted operation rejected: %v", err)
	}
}
