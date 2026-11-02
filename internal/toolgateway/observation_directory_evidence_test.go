package toolgateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakj-go/Argus/internal/observationscope"
)

func writeDiscoveryFilterEvidence(t *testing.T, firstInput observationscope.DiscoveryInput, first, second observationscope.DiscoveryPage, emptyInput observationscope.DiscoveryInput, empty observationscope.DiscoveryPage) {
	t.Helper()
	directory := os.Getenv("ARGUS_PLAN11_DISCOVERY_EVIDENCE")
	if directory == "" {
		return
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("discovery evidence directory must be absolute")
	}
	secondInput := firstInput
	secondInput.Cursor = first.NextCursor
	for _, tc := range []struct {
		name  string
		input observationscope.DiscoveryInput
		page  observationscope.DiscoveryPage
	}{{"first", firstInput, first}, {"second", secondInput, second}, {"empty", emptyInput, empty}} {
		raw, err := json.Marshal(map[string]any{"schema_version": "argus.plan11_discovery_filter_evidence/v1", "actual_postgres": true, "input": tc.input, "result": tc.page})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, "discovery-"+tc.name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("Actual PostgreSQL type/family/integration/name intersection precedes paging; three response pages sealed, cursor filter changes rejected")
}
