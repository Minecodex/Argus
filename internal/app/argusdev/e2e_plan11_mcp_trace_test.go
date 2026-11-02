package argusdev

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestCustomerMCPAcceptanceNeverAllowsNativeInvokeOrOtherConnectionTool(t *testing.T) {
	for _, raw := range []string{`[{"Tool":"tool.invoke"}]`, `[{"Tool":"connect"}]`, `[{"Tool":"mcp_other_write"}]`, `[]`} {
		if plan11CustomerMCPTraceAllowed(raw, "mcp_owned_read") == nil {
			t.Fatal("unrelated/native write or empty trace admitted", raw)
		}
	}
	if err := plan11CustomerMCPTraceAllowed(`[{"Tool":"tool.search"},{"Tool":"tool.describe"},{"Tool":"mcp_owned_read"}]`, "mcp_owned_read"); err != nil {
		t.Fatal(err)
	}
	if len(plan8SelectedModelNames("plan11-mcp")) != 6 || !plan8StageSelected("plan11-mcp", "plan11-customer-mcp") || plan8StageSelected("plan11-mcp", "plan11-chat-interactions") || plan8StageSelected("plan11-interactions", "plan11-customer-mcp") {
		t.Fatal("MCP acceptance changed its declared six independent tasks")
	}
}

func TestPostgresCustomerMCPSnapshotEvidenceUsesCurrentSchema(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var raw []byte
	if err = store.Pool.QueryRow(t.Context(), plan11CustomerMCPSnapshotSQL(uuid.NewString())).Scan(&raw); err != nil || !json.Valid(raw) || string(raw) != "{}" {
		t.Fatalf("current Run snapshot export query failed: %s %v", raw, err)
	}
}
