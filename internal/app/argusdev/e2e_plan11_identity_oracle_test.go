package argusdev

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestPostgresIdentityOracleAcceptsEquivalentAuthorizedPlacementShapes(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var empty []byte
	if err := store.Pool.QueryRow(t.Context(), plan11MetadataModelLedgerSQL(uuid.NewString())).Scan(&empty); err != nil || string(empty) != "[]" {
		t.Fatal("current per-Run usage-only ledger query failed", err)
	}
	run, collector, host, other := uuid.NewString(), uuid.New(), uuid.NewString(), uuid.NewString()
	for _, route := range []struct{ category, name string }{{"monitoring", "collector.choices"}, {"connector", "collector.list"}, {"connector", "collector.get"}} {
		for _, matching := range []bool{true, false} {
			collectorItem := map[string]any{"id": collector.String(), "resource_type": "host", "resource_id": host}
			if route.category == "monitoring" {
				collectorItem = map[string]any{"id": collector.String(), "resource": map[string]any{"kind": "host", "id": host}}
			}
			items := []any{collectorItem}
			if !matching {
				// Both IDs still occur, but in different records. A textual
				// contains check would wrongly accept this false relationship.
				if route.category == "monitoring" {
					collectorItem["resource"] = map[string]any{"kind": "host", "id": other}
					items = append(items, map[string]any{"id": other, "resource": map[string]any{"kind": "host", "id": host}})
				} else {
					collectorItem["resource_id"] = other
					items = append(items, map[string]any{"id": other, "resource_type": "host", "resource_id": host})
				}
			}
			summary := map[string]any{"items": items}
			if route.name == "collector.get" {
				summary = map[string]any{"collector": collectorItem}
			}
			input, _ := json.Marshal(map[string]any{"category": route.category, "name": route.name})
			projection, _ := json.Marshal(map[string]any{"summary": summary})
			cte := `WITH tool_calls AS(SELECT $1::uuid id,$2::uuid enterprise_id,$3::uuid run_id,'succeeded'::text status,'argus'::text source,'tool.invoke'::text tool_id,$4::jsonb input),tool_results AS(SELECT $1::uuid tool_call_id,$2::uuid enterprise_id,$5::jsonb projection) `
			var count int
			if err := store.Pool.QueryRow(t.Context(), cte+plan11PlacementReadSQL(run, collector, host), uuid.New(), uuid.New(), run, input, projection).Scan(&count); err != nil || (count == 1) != matching {
				t.Fatal("equivalent placement or same-record boundary failed", route, matching, count, err)
			}
		}
	}
}

func TestIdentityAlternativeRouteReplaysOriginalReadTrace(t *testing.T) {
	path := os.Getenv("ARGUS_PLAN11_IDENTITY_REPLAY")
	if path == "" {
		t.Skip("optional sealed original trace replay")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Cases []struct{ Calls json.RawMessage } `json:"cases"`
	}
	if json.Unmarshal(raw, &proof) != nil || len(proof.Cases) != 1 || len(proof.Cases[0].Calls) == 0 {
		t.Fatal("original single-task failed trace missing")
	}
	if err := plan11IdentityTraceAllowed(string(proof.Cases[0].Calls)); err != nil {
		t.Fatal("actual authorized alternative metadata route still rejected", err)
	}
}
