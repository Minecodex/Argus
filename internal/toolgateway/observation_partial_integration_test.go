package toolgateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

// Keep the resource/source authority backend healthy while independently closing
// the Dashboard pool. This is a real unavailable backend, not an injected return.
func verifyCapabilityBackendFailureIsPartial(t *testing.T, address string, store *postgres.Store, actor authorization.Actor, tools ObservationTools, role, granted, hidden uuid.UUID) {
	t.Helper()
	call := mcp.Call{Input: map[string]any{
		"resources": []resourceref.Ref{{Type: "monitoring_target", ID: granted}},
		"sections":  []string{"sources", "dashboards", "next_tools"},
	}}
	denied, err := tools.capabilities(t.Context(), actor, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(t.Context(), "INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read')", role); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// The existing test still verifies revocation after this helper returns.
		if _, err := store.Pool.Exec(t.Context(), "DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='telemetry.dashboard.read'", role); err != nil {
			t.Error(err)
		}
	}()
	closed, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	tools.Dashboard.Store = closed
	unavailable, err := tools.capabilities(t.Context(), actor, call)
	if err != nil {
		t.Fatalf("one failed capability section aborted all authorized sections: %v", err)
	}
	cases := []struct {
		name string
		data any
	}{
		{"not_permitted", denied}, {"unavailable", unavailable},
	}
	for _, tc := range cases {
		data := tc.data.(map[string]any)
		if sampled, ok := data["sampling_performed"].(bool); !ok || sampled || data["data_state"] != "not_checked" {
			t.Fatal("partial metadata result lost this request's no-sampling activity fact")
		}
		items := data["items"].([]any)
		if data["complete"] != false || len(items) != 1 {
			t.Fatal("partial capability result lost its explicit completeness or authorized resource")
		}
		sections := items[0].(map[string]any)["sections"].(map[string]capabilitySection)
		part := sections["dashboards"]
		rawItems, err := json.Marshal(part.Items)
		if err != nil || part.Status != tc.name || part.Complete || string(rawItems) != "[]" {
			t.Fatalf("%s section leaked items or claimed completeness: %+v", tc.name, part)
		}
		if sections["sources"].Status != "available" || !sections["sources"].Complete || sections["next_tools"].Status != "available" {
			t.Fatal("unaffected capability sections were mislabeled as unavailable or empty")
		}
		raw, err := json.Marshal(data)
		if err != nil || strings.Contains(string(raw), hidden.String()) || strings.Contains(string(raw), "closed pool") || strings.Contains(string(raw), "\"count\"") {
			t.Fatal("partial capability response leaked an unauthorized identity, count or internal backend error")
		}
		if directory := os.Getenv("ARGUS_PLAN11_CAPABILITY_EVIDENCE"); directory != "" {
			if !filepath.IsAbs(directory) {
				t.Fatal("capability evidence directory must be absolute")
			}
			if err = os.WriteFile(filepath.Join(directory, "capability-"+tc.name+".json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Log("Actual capability responses distinguish not_permitted and unavailable, retain healthy source/navigation sections, and expose no hidden identity/count/backend error")
}
