package telemetry

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestClaimAndNodeBindingSQLPreservesIsolationAndEvidenceState(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration := readTestFile(t, filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	queries := readTestFile(t, filepath.Join(root, "internal", "storage", "postgres", "queries", "telemetry_control.sql"))
	for _, required := range []string{
		"CREATE UNIQUE INDEX collection_claims_active_primary ON public.collection_claims",
		"FOREIGN KEY (primary_claim_id, enterprise_id) REFERENCES public.collection_claims(id, enterprise_id)",
		"CREATE UNIQUE INDEX collection_claims_active_migration_per_collector",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("M7 Claim migration is missing invariant %q", required)
		}
	}
	for _, required := range []string{
		"THEN kubernetes_node_host_bindings.host_id ELSE EXCLUDED.host_id END",
		"AND status = 'proposed'",
		"physical_resource_ref = 'host:' || sqlc.narg('physical_resource_ref')",
		"physical_resource_ref = 'kubernetes_cluster:' || sqlc.narg('physical_resource_ref')",
	} {
		if !strings.Contains(queries, required) {
			t.Fatalf("M7 Claim/Binding query is missing invariant %q", required)
		}
	}
}

func TestHelmIncludesEveryClickHouseMigration(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	source := filepath.Join(root, "migrations", "clickhouse")
	chart := filepath.Join(root, "deploy", "helm", "argus-telemetry-pipeline")
	paths, err := filepath.Glob(filepath.Join(source, "*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatal("no authoritative ClickHouse migrations", err)
	}
	names := []string{}
	var latest uint64
	for _, path := range paths {
		name := filepath.Base(path)
		names = append(names, name)
		sql := readTestFile(t, path)
		if sql != readTestFile(t, filepath.Join(chart, "files", name)) {
			t.Fatalf("Helm migration %s differs from authority", name)
		}
		for _, match := range regexp.MustCompile(`(?i)SELECT\s+(\d+)\s+WHERE\s+NOT\s+EXISTS`).FindAllStringSubmatch(sql, -1) {
			version, err := strconv.ParseUint(match[1], 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			latest = max(latest, version)
		}
	}
	if latest != uint64(TelemetrySchemaVersion) {
		t.Fatalf("migration version %d differs from runtime requirement %d", latest, TelemetrySchemaVersion)
	}
	packaged, err := filepath.Glob(filepath.Join(chart, "files", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range packaged {
		packaged[i] = filepath.Base(packaged[i])
	}
	if !slices.Equal(names, packaged) {
		t.Fatal("Helm migration set differs", names, packaged)
	}
	template := readTestFile(t, filepath.Join(chart, "templates", "migration.yaml"))
	if !strings.Contains(template, `.Files.Glob "files/*.sql"`) || !strings.Contains(template, `for migration in /migrations/*.sql`) {
		t.Fatal("Helm does not package and execute the full ordered migration set")
	}
}

func TestTelemetrySchemaV3UsesTenantBootstrap(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration := readTestFile(t, filepath.Join(root, "migrations", "clickhouse", "00001_m7_telemetry.sql"))
	for _, required := range []string{"Schema v3", "SELECT 3 WHERE NOT EXISTS", "schema_versions", "metric_series_local"} {
		if !strings.Contains(migration, required) {
			t.Fatalf("telemetry schema v3 is missing invariant %q", required)
		}
	}
	chartMigration := readTestFile(t, filepath.Join(root, "deploy", "helm", "argus-telemetry-pipeline", "files", "00001_m7_telemetry.sql"))
	if migration != chartMigration {
		t.Fatal("ClickHouse migration and Helm schema copy differ")
	}
}

func TestTelemetryQueryRoleHasOnlyRequiredTenantLifecycleWrites(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration := readTestFile(t, filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	for _, required := range []string{
		"GRANT SELECT, INSERT, UPDATE ON public.enterprise_telemetry_tables TO argus_telemetry_query",
		"GRANT SELECT, INSERT, UPDATE ON public.audit_chain_heads TO argus_telemetry_query",
		"GRANT INSERT ON public.audit_events TO argus_telemetry_query",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("M10 query role migration is missing %q", required)
		}
	}
	if strings.Contains(migration, "GRANT DELETE ON public.enterprise_telemetry_tables TO argus_telemetry_query") {
		t.Fatal("M10 query role unexpectedly has tenant readiness delete permission")
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
