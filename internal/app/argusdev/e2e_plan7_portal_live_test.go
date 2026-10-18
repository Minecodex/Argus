//go:build plan7portal

package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This opt-in acceptance uses an explicitly supplied existing account and the
// deployed production API. It never invokes an installer, resets a tenant or
// alters the account's credentials, MFA, role bindings or Workspace settings.
// Actual chats and the owned Docker targets remain available for user review.
func TestLivePlan7ExistingPortal(t *testing.T) {
	if os.Getenv("ARGUS_PLAN7_PORTAL") != "1" {
		t.Skip("requires an explicitly authorized local portal account")
	}
	root, err := findRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	runID := os.Getenv("ARGUS_PLAN7_PORTAL_RUN")
	if !windowsE2EIdentifier.MatchString(runID) || os.Getenv("ARGUS_PLAN7_PORTAL_USER") == "" || os.Getenv("ARGUS_PLAN7_PORTAL_PASSWORD") == "" {
		t.Fatal("explicit account and unique run ID are required")
	}
	artifacts := filepath.Join(root, "build/plan7-evidence", runID)
	if err = os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	model, err := loadP5RealModelConfig(filepath.Join(root, "build/plan7-apimart-model.json"))
	if err != nil {
		t.Fatal(err)
	}
	model.EndpointIP = "" // The deployed endpoint policy uses its normal public DNS.
	kube, err := NewE2EKube("docker-desktop", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{root: root, stdout: os.Stdout, stderr: os.Stderr, runner: Runner{Dir: root, Stdout: os.Stdout, Stderr: os.Stderr}}
	env := &E2EEnvironment{Root: root, Options: E2EOptions{Suite: "plan7", Plan7Phase: "linux-chatbox", RunID: runID, Artifacts: artifacts, RealModel: model, KubeContext: "docker-desktop"}, ConfigPath: filepath.Join(root, "deploy/.cache/argus-install-consolidated.yaml"), SystemNS: "argus-system", SandboxNS: "argus-sandbox", ObservNS: "argus-system", IngressNS: "argus-ingress", ReleaseID: "portal-" + runID, ImagePlatform: "linux/amd64", Kube: kube, State: NewScenarioState(runID)}
	env.Endpoints = &E2EEndpoints{EnterpriseOrigin: "https://argus.dev", PlatformOrigin: "https://platform.argus.dev", APIBase: "https://argus.dev/api/v1", IngressDialAddress: "127.0.0.1:443"}
	env.State.HTTP = NewDomainScenarioHTTP(env)
	env.State.Values["enterprise_username"] = os.Getenv("ARGUS_PLAN7_PORTAL_USER")
	env.State.Values["enterprise_password"] = os.Getenv("ARGUS_PLAN7_PORTAL_PASSWORD")
	env.State.Values["host_ingress_service"] = "argus-ingress"
	env.State.Values["p6_backend_build"] = "production"
	env.State.Values["existing_portal_account"] = "true"
	var releaseProof struct {
		Status    string `json:"status"`
		Artifacts []struct {
			URI    string `json:"uri"`
			SHA    string `json:"sha256"`
			Signed bool   `json:"signature_verified"`
		} `json:"artifacts"`
	}
	proof, err := os.ReadFile(filepath.Join(root, "build/local-release-plan7-20261017/artifact-download-verification.json"))
	if err != nil || json.Unmarshal(proof, &releaseProof) != nil || releaseProof.Status != "passed" {
		t.Fatal("verified local release download receipt required")
	}
	for _, artifact := range releaseProof.Artifacts {
		if artifact.Signed && strings.Contains(artifact.URI, "/argus-otelcol/") && strings.HasSuffix(artifact.URI, "/linux-amd64.tar.gz") {
			env.State.Values["portal_linux_collector_sha"] = artifact.SHA
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Minute)
	defer cancel()
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		t.Fatal(err)
	}
	session, err := env.State.HTTP.JSON(ctx, "portal-owner", "enterprise", http.MethodGet, "/enterprise/auth/session", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		t.Fatal(err)
	}
	username, _ := nestedString(session, "user", "username")
	if username != env.State.Values["enterprise_username"] {
		t.Fatal("portal account mismatch")
	}
	env.State.Values["admin_user_id"], err = stringField(session, "user", "id")
	if err != nil {
		t.Fatal(err)
	}
	env.State.Values["enterprise_id"], err = stringField(session, "user", "enterprise_id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if id := env.State.Values["p7_real_model_id"]; uuid.Validate(id) == nil {
			query := fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT c.id,c.run_id,c.call_kind,c.status,c.error_code,c.input_tokens,c.output_tokens,c.input_usage_source,c.output_usage_source,c.latency_ms,EXISTS(SELECT 1 FROM model_quota_reservations q WHERE q.model_call_id=c.id AND q.status='settled' AND q.usage_source='provider') AS quota_settled FROM model_calls c WHERE c.model_id='%s' AND c.enterprise_id='%s' ORDER BY c.created_at) t`, id, env.State.Values["enterprise_id"])
			cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
			defer stop()
			if raw, e := a.postgresQuery(cleanup, env, query); e == nil {
				_ = writePrivate(filepath.Join(artifacts, "portal-model-calls.json"), []byte(strings.TrimSpace(raw)))
			}
		}
		public := map[string]any{"run_id": runID, "account": username, "model": model.ModelID, "status": "failed", "scope": "existing-account Linux workflow; not the full cross-platform matrix", "complete": false, "history_retained": true, "model_id": env.State.Values["p7_real_model_id"], "telemetry_conversation": env.State.Values["p7_telemetry_conversation"], "repair_conversation": env.State.Values["p7_model_conversation"], "recovery_conversation": env.State.Values["p7_recovery_conversation"]}
		if !t.Failed() {
			public["status"] = "passed"
		}
		raw, _ := json.MarshalIndent(public, "", "  ")
		if e := writePrivate(filepath.Join(artifacts, "portal-result.json"), raw); e != nil {
			t.Error(e)
		}
	}()
	resourceArtifacts := artifacts
	if reuse := os.Getenv("ARGUS_PLAN7_PORTAL_REUSE_RUN"); reuse != "" {
		if !windowsE2EIdentifier.MatchString(reuse) || reuse == runID {
			t.Fatal("invalid prior owned resource run")
		}
		resourceArtifacts = filepath.Join(root, "build/plan7-evidence", reuse)
	}
	fixturePath := filepath.Join(resourceArtifacts, "plan7-docker-ownership.json")
	var fixture *plan6DockerFixture
	if _, e := os.Stat(fixturePath); e == nil {
		env.Options.Plan6HostFixtures = fixturePath
		fixture, err = a.loadPlan6DockerFixture(ctx, fixturePath)
	} else {
		fixture, err = a.createPlan7DockerFixture(ctx, env)
	}
	if err != nil {
		t.Fatal(err)
	}
	host := &plan6HostLoop{Docker: fixture, HostName: "AIOps Linux 实测 · glm-5.3-flash · " + runID, CPUExpression: `100 * (1 - avg by (argus_resource_id,argus_source_id) (system_cpu_utilization{state="idle"}))`, MemoryExpression: `100 * system_memory_utilization{state="used"}`}
	statePath := filepath.Join(artifacts, "portal-resource-state.json")
	var saved struct {
		Host    *plan6HostLoop `json:"host"`
		ModelID string         `json:"model_id"`
	}
	if raw, e := os.ReadFile(filepath.Join(resourceArtifacts, "portal-resource-state.json")); e == nil {
		if json.Unmarshal(raw, &saved) != nil || saved.Host == nil {
			t.Fatal("invalid owned portal state")
		}
		host = saved.Host
		host.Docker = fixture
		env.State.Values["p7_real_model_id"] = saved.ModelID
	}
	if resourceArtifacts != artifacts {
		current, e := env.State.HTTP.JSON(ctx, "portal-reused-host", "enterprise", http.MethodGet, "/enterprise/hosts/"+host.HostID, 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			t.Fatal(e)
		}
		if current["address"] != fixture.HostIP {
			t.Fatal("prior owned Host no longer matches the Docker fixture")
		}
	}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"host": host, "model_id": env.State.Values["p7_real_model_id"]}, "", "  ")
		_ = writePrivate(statePath, raw)
	}()
	if host.HostID == "" {
		t.Log("formal Linux onboarding started")
		if err = a.plan6HostOnboarding(ctx, env, host); err != nil {
			t.Fatal(err)
		}
	} else if host.SourceID == "" {
		t.Log("formal Linux Collector enrollment resumed")
		if err = a.plan6HostCollector(ctx, env, host); err != nil {
			t.Fatal(err)
		}
	}
	if host.DashboardID == "" {
		t.Log("formal live telemetry started")
		if err = a.plan6HostMetricsDashboard(ctx, env, host); err != nil {
			t.Fatal(err)
		}
	}
	for _, phase := range []struct {
		name string
		run  func() error
	}{
		{"real-browser-diagnosis-and-repair", func() error { return a.plan7LinuxChatbox(ctx, env, host) }},
		{"two-stage-evidence", func() error { return a.plan7ModelEvidenceSequence(ctx, env, host) }},
		{"lost-context-recovery", func() error { return a.plan7ModelRecovery(ctx, env, host) }},
		{"private-concurrent-context", func() error { return a.plan7ModelActivity(ctx, env, host) }},
	} {
		t.Log(phase.name, "started")
		if err = phase.run(); err != nil {
			t.Fatal(err)
		}
		if err = plan7WritePhase(env, "portal-"+phase.name); err != nil {
			t.Fatal(err)
		}
		t.Log(phase.name, "passed")
	}
}

func TestSavePlan7BudgetSource(t *testing.T) {
	stage := os.Getenv("ARGUS_PLAN7_FREEZE_STAGE")
	if stage == "" {
		t.Skip("explicit local source snapshot only")
	}
	root, err := findRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "build/local-release-plan7-budget-20261017")
	a := &App{root: root, stdout: os.Stdout, stderr: os.Stderr, runner: Runner{Dir: root, Stdout: os.Stdout, Stderr: os.Stderr}}
	env := &E2EEnvironment{Root: root, State: NewScenarioState("plan7-budget-20261017-r01"), Options: E2EOptions{RunID: "plan7-budget-20261017-r01", Artifacts: artifacts}}
	if _, err = a.plan6Source(t.Context(), env, stage); err != nil {
		t.Fatal(err)
	}
}
