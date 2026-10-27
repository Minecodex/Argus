package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) runPlan7Scenario(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.RealModel == nil {
		return fmt.Errorf("Plan7 requires a real model")
	}
	if err := a.plan7WorkspaceDevicePrerequisites(ctx, env); err != nil {
		return err
	}
	if err := a.configureP5Workspace(ctx, env); err != nil {
		return err
	}
	env.State.Values["host_prerequisites_ready"] = "true"
	if err := a.preparePlan7ControlModel(ctx, env); err != nil {
		return err
	}
	fixture, err := a.createPlan7DockerFixture(ctx, env)
	if err != nil {
		return err
	}
	host := &plan6HostLoop{Docker: fixture, HostName: "Plan7 Docker monitored Host", CPUExpression: `100 * (1 - avg by (argus_resource_id,argus_source_id,argus_collection_id) (system_cpu_utilization{state="idle"}))`, MemoryExpression: `100 * system_memory_utilization{state="used"}`}
	for _, phase := range []struct {
		name string
		run  func() error
	}{
		{"workspace-offline-boundary", func() error {
			conversation, err := a.p5Conversation(ctx, env, "Plan7 offline Workspace boundary")
			if err != nil {
				return err
			}
			return a.p5BashProof(ctx, env, conversation, "plan7-workspace-network", p5NetworkProof)
		}},
		{"linux-onboarding-and-collector", func() error { return a.plan6HostOnboarding(ctx, env, host) }},
		{"live-telemetry", func() error { return a.plan6HostMetricsDashboard(ctx, env, host) }},
		{"linux-confirmations-and-files", func() error { return a.plan7LinuxControlFlow(ctx, env, host.HostID) }},
		{"windows-enrollment-and-powershell", func() error {
			id, err := a.plan7WindowsEnrollment(ctx, env)
			if err != nil {
				return err
			}
			return a.plan7WindowsControlFlow(ctx, env, id)
		}},
		{"kubernetes-client-and-exec", func() error { return a.plan7KubernetesControlFlow(ctx, env) }},
		{"kubernetes-direct-executor", func() error { return a.plan7DirectKubernetes(ctx, env) }},
		{"durable-worker-fault", func() error { return a.plan7WorkerFault(ctx, env, host) }},
		{"real-browser", func() error { return a.plan7Browser(ctx, env, host.HostID) }},
		{"production-backend", func() error { return a.usePlan6ProductionBackend(ctx, env) }},
		{"real-model-diagnosis-and-repair", func() error { return a.plan7RealModelFlow(ctx, env, host) }},
		{"real-model-two-stage-evidence", func() error { return a.plan7ModelEvidenceSequence(ctx, env, host) }},
		{"real-model-lost-context-recovery", func() error { return a.plan7ModelRecovery(ctx, env, host) }},
		{"real-model-platforms", func() error { return a.plan7ModelPlatforms(ctx, env) }},
		{"real-model-evidence-quality", func() error { return a.plan7ModelEvidenceQuality(ctx, env, host) }},
		{"real-model-concurrent-context", func() error { return a.plan7ModelActivity(ctx, env, host) }},
	} {
		if env.Options.Plan7Phase == "linux-chatbox" {
			switch phase.name {
			case "windows-enrollment-and-powershell", "kubernetes-client-and-exec", "kubernetes-direct-executor", "durable-worker-fault", "real-browser", "real-model-platforms", "real-model-evidence-quality":
				continue
			case "real-model-diagnosis-and-repair":
				phase.run = func() error { return a.plan7LinuxChatbox(ctx, env, host) }
			}
		}
		if phase.name == "durable-worker-fault" && env.Options.Plan7Phase == "flow" {
			continue
		}
		_, _ = fmt.Fprintln(a.stdout, "Plan7", phase.name, "started")
		if err = phase.run(); err != nil {
			return fmt.Errorf("Plan7 %s: %w", phase.name, err)
		}
		if err = plan7WritePhase(env, phase.name); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(a.stdout, "Plan7", phase.name, "passed")
	}
	if env.Options.Plan7Phase != "all" {
		return nil
	}
	if err = a.plan7Engineering(ctx, env, host); err != nil {
		return err
	}
	if err = plan7WritePhase(env, "engineering"); err != nil {
		return err
	}
	if err = buildPlan7Matrix(ctx, env); err != nil {
		return err
	}
	return verifyPlan7Matrix(env)
}

// The complete gate cannot be inferred from a successful happy-path run. Each
// matrix item requires evidence produced for this same frozen acceptance run.
func verifyPlan7Matrix(env *E2EEnvironment) error {
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan7-matrix.json"))
	if err != nil {
		return fmt.Errorf("Plan7 full acceptance requires all 57 matrix evidence records: %w", err)
	}
	var report struct {
		RunID  string `json:"run_id"`
		Source string `json:"source_fingerprint"`
		Cases  []struct {
			ID       string                        `json:"id"`
			Status   string                        `json:"status"`
			Evidence []struct{ Path, Hash string } `json:"evidence"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &report) != nil || report.RunID != env.Options.RunID || report.Source != env.State.Values["p6_source_fingerprint"] {
		return fmt.Errorf("Plan7 evidence is not from this frozen run")
	}
	required := map[string]bool{}
	for _, group := range []struct {
		prefix string
		count  int
	}{{"A", 38}, {"M", 11}, {"U", 8}} {
		for n := 1; n <= group.count; n++ {
			required[fmt.Sprintf("P7-%s%02d", group.prefix, n)] = true
		}
	}
	seen := map[string]bool{}
	for _, entry := range report.Cases {
		if !required[entry.ID] || seen[entry.ID] || entry.Status != "passed" || len(entry.Evidence) == 0 {
			return fmt.Errorf("Plan7 invalid or incomplete matrix item %s", entry.ID)
		}
		seen[entry.ID] = true
		for _, proof := range entry.Evidence {
			if filepath.IsAbs(proof.Path) || !filepath.IsLocal(proof.Path) || strings.Contains(proof.Path, "\\") {
				return fmt.Errorf("Plan7 evidence path must be relative to this run")
			}
			data, e := os.ReadFile(filepath.Join(env.Options.Artifacts, filepath.FromSlash(proof.Path)))
			if e != nil {
				return e
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != proof.Hash {
				return fmt.Errorf("Plan7 evidence hash changed for %s", entry.ID)
			}
		}
	}
	if len(seen) != 57 {
		return fmt.Errorf("Plan7 full matrix has %d/57 passed items", len(seen))
	}
	return nil
}
