package argusdev

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlan8NamedEvidenceRejectsMissingSkippedAndUnrelatedPasses(t *testing.T) {
	for _, line := range []string{`{"Action":"pass","Package":"fixture"}`, `{"Action":"skip","Test":"Required"}`, `{"Action":"pass","Test":"Required/subtest"}`, `{"Action":"pass","Test":"Other"}`} {
		log := newPlan8EventLog(io.Discard, []string{"Required"})
		if _, err := log.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
		if log.finish() == nil {
			t.Fatalf("incomplete evidence passed: %s", line)
		}
	}
	log := newPlan8EventLog(io.Discard, []string{"Required"})
	_, _ = log.Write([]byte(`{"Action":"pass","Test":"Required"}`))
	if err := log.finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPlan8EvidenceMasksCredentialsBeforeWritingChunkedLogs(t *testing.T) {
	var out bytes.Buffer
	log := newPlan8EventLog(&out, nil)
	event, _ := json.Marshal(map[string]any{"Action": "output", "Output": "password=private-canary api_key=private-key"})
	for _, part := range [][]byte{event[:20], event[20:], []byte("\n")} {
		if _, err := log.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "private-canary") || strings.Contains(out.String(), "private-key") || !strings.Contains(out.String(), "REDACTED") {
		t.Fatal("raw credential reached evidence writer")
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestPlan8SnapshotIncludesUncommittedCodeButIgnoresEvidence(t *testing.T) {
	root := t.TempDir()
	r := Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}
	for _, args := range [][]string{{"init", "-b", "codex/plan8-fixture"}, {"-c", "user.name=Plan8 fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "Plan8 snapshot fixture"}} {
		if err := r.Run(t.Context(), nil, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	app := App{root: root, runner: r}
	before, err := app.plan8Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "untracked.go"), []byte("package fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := app.plan8Snapshot(t.Context())
	if err != nil || after.Fingerprint == before.Fingerprint {
		t.Fatal("uncommitted implementation omitted from source binding")
	}
	if err = os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "artifacts", "evidence.log"), []byte("test evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	last, err := app.plan8Snapshot(t.Context())
	if err != nil || after.Fingerprint != last.Fingerprint {
		t.Fatal("evidence changed its own source fingerprint")
	}
}

func TestPlan8BackendAdmissionRejectsBusinessAndRemoteTargetsWithoutLeakingURL(t *testing.T) {
	for _, value := range []string{"postgres://postgres:private-canary@production.example.com/argus_plan8", "postgres://postgres:private-canary@127.0.0.1:5432/business", "postgres://postgres@127.0.0.1/argus_plan8"} {
		_, _, err := plan8BackendTarget("ARGUS_MONITORING_TEST_DATABASE_URL", value)
		if err == nil || strings.Contains(err.Error(), "private-canary") {
			t.Fatal("unsafe target or credential disclosure")
		}
	}
	if port, binding, err := plan8BackendTarget("ARGUS_MONITORING_TEST_DATABASE_URL", "postgres://postgres@127.0.0.1:64606/argus_plan8"); err != nil || port != "5432/tcp" || binding != "64606" {
		t.Fatal("explicit disposable address rejected")
	}
	if _, _, err := plan8BackendTarget("ARGUS_CLICKHOUSE_TEST_ADDRESS", "remote.example.com:9000"); err == nil {
		t.Fatal("remote backend accepted")
	}
}

func TestPlan8SuiteRegistryKeepsEngineeringBeforeOtherMutatingChecks(t *testing.T) {
	checks, err := plan8Checks("full", t.TempDir())
	if err != nil || len(checks) != 90 || checks[0].ID != "contracts" {
		t.Fatal("generation must finish before frontend/backend checks")
	}
	for _, check := range checks[1:] {
		if len(check.Tests) == 0 {
			t.Fatal("component check lacks executed named tests")
		}
	}
	if checks[2].ID != "authorization-registry-upgrade" || len(checks[2].Tests) != 7 {
		t.Fatal("existing-enterprise registry upgrade lacks its actual database gate")
	}
	if checks[3].ID != "model-record-concurrency" || len(checks[3].Tests) != 5 {
		t.Fatal("model record transaction conflict regressions lack their actual database gate")
	}
	if checks[4].ID != "metric-scan-pruning" || len(checks[4].Tests) != 1 {
		t.Fatal("real selected-series scan gate is missing")
	}
	if checks[5].ID != "runtime-database-roles" || len(checks[5].Tests) != 5 || !strings.Contains(strings.Join(checks[5].Args, " "), "./internal/app/argusctl") {
		t.Fatal("runtime database role regression must run before native resource checks")
	}
	for _, name := range []string{"TestPostgresTelemetryRuntimeRolesAndLateRoleReconciliation", "TestPostgresTelemetryEnrollmentDeadlineUsesOneDatabaseClock", "TestPostgresTelemetryEnrollmentLifetimeBoundsAndRollback", "TestPostgresTelemetryEnrollmentConsumptionUsesCurrentDatabaseTime", "TestCollectorPKIRepairUsesDatabaseDeadlineAndRollsBackRevocation"} {
		if !strings.Contains(" "+strings.Join(checks[5].Tests, " ")+" ", " "+name+" ") {
			t.Fatal("runtime database role gate lost a required clock or repair regression", name)
		}
	}
	if checks[6].ID != "installer-bootstrap" || len(checks[6].Tests) != 2 || checks[6].Env["ARGUS_PLAN8_HELM_BOOTSTRAP"] != "1" || checks[6].Env["ARGUS_PLAN8_INSTALL_TOOLS_LIVE"] != "1" {
		t.Fatal("actual Helm bootstrap handoff and ordinary rollback must run before native resource checks")
	}
	for _, check := range checks {
		if check.ID == "connector-disconnect" {
			if len(check.Tests) != 12 || !strings.Contains(strings.Join(check.Args, " "), "./internal/app/connector") {
				t.Fatal("disconnect gate must retain PostgreSQL fences and bounded client sessions")
			}
			for _, name := range []string{
				"TestPostgresDisconnectProjectsAtomicallyBeforeNewSession",
				"TestPostgresGatewayMonitoringDispatchUsesConfiguredPrivateCredentialChannel",
				"TestPostgresHeartbeatFencesEveryProjectionAndRollsBackInvalidFrames",
				"TestPostgresCollectorFailurePreservesOnlyVerifiedEffectiveRoute",
				"TestPostgresServerRejectsUndeliveredCommandsWithoutOpeningClientTransition",
				"TestPostgresCredentialDependencyFailuresStayRetryable",
				"TestPostgresCollectorCommandExpirationProjectsFailedAndUnknownOutcomes",
				"TestPostgresCollectorReconnectUsesVerifiedReceiptAndRejectsStaleOrForgedResults",
				"TestPostgresCollectorReconnectCannotOverwriteNewIntent",
				"TestControlSessionTimesOutForSilentGateway",
				"TestControlSessionRequiresAdvancingAcknowledgements",
				"TestControlHandshakeHasBoundedWelcomeWait",
			} {
				if !strings.Contains(" "+strings.Join(check.Tests, " ")+" ", " "+name+" ") {
					t.Fatal("disconnect gate lost a bounded control-session regression", name)
				}
			}
		}
		if check.ID == "connector-rotation" && (len(check.Tests) != 1 || check.Tests[0] != "TestCertificateRotationPersistsIdentityUsableForSubsequentRepair") {
			t.Fatal("certificate rotation must preserve a usable CSR for later repair")
		}
		if check.ID == "contracts" && check.Env["CI"] != "1" {
			t.Fatal("isolated generation must suppress interactive notifier behavior")
		}
		if check.ID == "tools" && !strings.Contains(strings.Join(check.Tests, " "), "TestUndescribedArgumentsCannotBypassDiscoveryError") {
			t.Fatal("native discovery must retain the prerequisite-before-business-schema regression")
		}
		if check.ID == "context-runtime-wire" && (len(check.Tests) != 2 || !strings.Contains(strings.Join(check.Args, " "), "-tags=m4e2e")) {
			t.Fatal("runtime request and durable budget acceptance must execute the explicit controlled model cases")
		}
		if check.ID == "postgres-backup-restore" && (len(check.Tests) != 4 || check.Env["ARGUS_PLAN8_KUBECTL_TRANSFER"] != "1" || check.Env["ARGUS_PLAN8_OPENBAO_RECOVERY"] != "1" || check.Env["ARGUS_PLAN8_PKI_RECOVERY"] != "1") {
			t.Fatal("backup acceptance must exercise actual cross-drive Pod binary and archive transfer")
		}
	}
	if _, err = plan8Checks("pretend-full", ""); err == nil {
		t.Fatal("unknown scope silently accepted")
	}
}
