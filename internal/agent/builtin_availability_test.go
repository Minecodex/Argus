package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestBuiltinAvailabilityUsesActualCatalogAndExplainsSnapshotStatus(t *testing.T) {
	f := newRecoveryFixture(t)
	// A stale definition must not advertise a tool absent from this request.
	f.exec(`UPDATE runs SET tool_snapshot='{"sandbox_status":"quota_unavailable","tools":[{"model":{"name":"bash"}}]}'::jsonb WHERE id=$1`, f.r)
	loop := Loop{Store: f.store}
	run := f.run()
	messages, input, _, source, err := loop.messages(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	tools := []modelprovider.Tool{{Name: "tool.search"}, {Name: "connect"}, {Name: "external-private-name"}}
	projection, err := loop.context(t.Context(), run, db.AiModelRevision{ContextWindowTokens: 32768, MaxOutputTokens: 1024}, messages, input, tools, source)
	if err != nil {
		t.Fatal(err)
	}
	var facts struct {
		Builtins struct {
			Status    string   `json:"workspace_status_at_run_start"`
			Available []string `json:"available"`
		} `json:"builtin_tool_availability"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(projection.Messages[1].Content, "Server execution facts (data): ")), &facts); err != nil {
		t.Fatal(err)
	}
	if facts.Builtins.Status != "quota_unavailable" || !slices.Equal(facts.Builtins.Available, []string{"connect"}) {
		t.Fatalf("invented builtin availability: %+v", facts)
	}
	if !strings.Contains(projection.Messages[0].Content, "not entries in tool.search categories") {
		t.Fatal("provider instructions do not distinguish builtin and platform discovery")
	}
}
