package argusdev

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestPlan8ServiceDiagnosticsReadOnlyQueryMatchesActualSchema(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var raw []byte
	query := plan8ServiceOperationEvidenceSQL(uuid.New())
	if err = store.Pool.QueryRow(t.Context(), query).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var value map[string][]any
	if json.Unmarshal(raw, &value) != nil || len(value) != 4 {
		t.Fatal("operational evidence shape is incomplete")
	}
	for _, field := range []string{"plan,", "result,", "credential_id", "credential_lease_id", "target_address", "payload AS"} {
		if strings.Contains(query, field) {
			t.Fatal("diagnostic query includes private operational input")
		}
	}
}
