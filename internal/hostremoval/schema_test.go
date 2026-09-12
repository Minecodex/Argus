package hostremoval

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBaselineContainsRemovalFencesAndNoLegacyHostDeletePath(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	baseline, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(baseline)
	for _, expected := range []string{
		"CREATE TABLE public.host_removal_operations",
		"CREATE TABLE public.host_removal_operation_steps",
		"CREATE TABLE public.host_removal_operation_events",
		"CREATE TABLE public.host_removal_tokens",
		"CREATE UNIQUE INDEX host_removal_operations_active_target_idx",
		"removal_generation bigint DEFAULT 0 NOT NULL",
		"num_nonnulls(connector_command_id, telemetry_collector_operation_id, connector_install_operation_id, host_onboarding_operation_id, host_removal_operation_id)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("baseline is missing %q", expected)
		}
	}
	queries, err := os.ReadFile(filepath.Join(root, "internal", "storage", "postgres", "queries", "resources.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(queries), "-- name: DeleteHost :one\nUPDATE hosts SET status = 'deleted'") {
		t.Fatal("legacy unrestricted Host deletion query remains")
	}
	removalQueries, err := os.ReadFile(filepath.Join(root, "internal", "storage", "postgres", "queries", "host_removal_operations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	removalText := string(removalQueries)
	for _, dependency := range []string{"'telemetry_tunnel'::text", "'collector_operation'::text"} {
		if strings.Count(removalText, dependency) != 2 {
			t.Fatalf("Host and Bastion removal dependency inventories must both include %s", dependency)
		}
	}
}
