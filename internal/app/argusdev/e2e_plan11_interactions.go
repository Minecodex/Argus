package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type plan11InteractionCase struct {
	Directory string `json:"directory"`
	Port      int    `json:"port"`
}

func (a *App) plan11DeployedInteractions(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	if env.Options.Plan8Phase != "plan11-interactions" || env.Options.RealModel == nil || h == nil || h.Docker == nil || uuid.Validate(h.HostID) != nil {
		return errors.New("modern interaction acceptance requires its selected provider and owned enrolled Linux host")
	}
	proof := map[string]any{"schema_version": "argus.plan11_interactions/v1", "passed": false,
		"scope":          "current Markdown question/revision/approval/implementation/report and stop workflows; not legacy fixed-plan acceptance",
		"workflow_names": plan8SelectedModelNames("plan11-interactions"), "provider_model": env.Options.RealModel.ModelID,
		"worker_restart_tested": false, "whole_plan_complete": false}
	defer func() {
		if model, ok := proof["model_id"].(string); ok {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			failure = errors.Join(failure, a.plan11InteractionModelEvidence(cleanup, env, model, proof))
		}
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, err, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-interactions.json"), raw))
	}()
	binary := filepath.Join(env.Options.Artifacts, "plan11-status-server-linux-amd64")
	if err := a.runner.Run(ctx, map[string]string{"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"}, "go", "build", "-trimpath", "-o", binary, "./cmd/argus-e2e-status"); err != nil {
		return err
	}
	if _, err := a.plan6InspectDocker(ctx, h.Docker.Host, h.Docker.Owner); err != nil {
		return err
	}
	if err := a.runner.Run(ctx, nil, "docker", "cp", binary, h.Docker.Host+":/usr/local/bin/argus-e2e-status"); err != nil {
		return err
	}
	if _, err := a.plan6DockerScript(ctx, h.Docker, h.Docker.Host, "chmod 0755 /usr/local/bin/argus-e2e-status\ntest -x /usr/local/bin/argus-e2e-status\n"); err != nil {
		return err
	}
	cases := map[string]plan11InteractionCase{}
	for index, access := range []string{"request_approval", "full_access"} {
		item := plan11InteractionCase{Directory: "/tmp/argus-chat-interactions-" + env.Options.RunID + "-" + access, Port: 18431 + index}
		if strings.ContainsAny(env.Options.RunID, "/\\.'\" ") {
			return errors.New("owned interaction fixture path is not a bounded run identity")
		}
		unit := "argus-p11-interaction-" + uuid.NewString() + ".service"
		output, err := a.plan6DockerScript(ctx, h.Docker, h.Docker.Host, plan11InteractionServiceScript(item, unit))
		if err != nil || !strings.Contains(output, "owned_http_503_ready") {
			return errors.Join(err, errors.New("owned interaction service did not establish its actual 503 precondition"))
		}
		cases[access] = item
	}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	proof["model_id"], proof["cases"] = model, cases
	fixture, _ := json.Marshal(map[string]any{"container": h.Docker.Host, "container_id": h.Docker.Host,
		"host_id": h.HostID, "model_id": model, "cases": cases})
	// Authentication is the same enterprise session used by the product. Existing
	// account handoff is handled by runPlaywright and never falls back to root.
	for _, spec := range []struct{ file, suffix string }{
		{"chat-interactions-real.spec.ts", "plan11-modern-interactions"},
		{"chat-interactions-stop-real.spec.ts", "plan11-modern-stop"},
	} {
		variables := map[string]string{"ARGUS_CHAT_INTERACTIONS_REAL": "1", "ARGUS_CHAT_INTERACTIONS_FIXTURE": string(fixture),
			"ARGUS_PLAN7_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLAN7_PASSWORD": env.State.Values["enterprise_password"],
			"ARGUS_E2E_ARTIFACT_SUFFIX": spec.suffix}
		if err := a.runPlaywright(ctx, env, spec.file, variables); err != nil {
			return err
		}
	}
	var reports []map[string]any
	for _, suffix := range []string{"plan11-modern-interactions", "plan11-modern-stop"} {
		root := filepath.Join(env.Options.Artifacts, "playwright-"+env.Options.Suite+"-"+suffix)
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || (entry.Name() != "real-interaction.json" && entry.Name() != "real-stop.json") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var report map[string]any
			if json.Unmarshal(raw, &report) != nil || report["status"] != "passed" {
				return errors.New("modern interaction browser report is not a completed workflow")
			}
			conversation, _ := report["conversation"].(string)
			if uuid.Validate(conversation) != nil {
				return errors.New("browser report has no actual conversation identity")
			}
			reports = append(reports, report)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(reports) != 4 {
		return fmt.Errorf("modern interaction browser workflows=%d, expected4", len(reports))
	}
	proof["reports"] = reports
	if err := a.plan11WaitModelAccounting(ctx, env); err != nil {
		return err
	}
	// All provider calls, including stopped and failed attempts, must retain
	// original usage. Do not infer accounting from the final browser report.
	stats, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('calls',count(*),'unknown_usage',count(*) FILTER(WHERE input_usage_source IS DISTINCT FROM 'provider' OR output_usage_source IS DISTINCT FROM 'provider'),'input_tokens',coalesce(sum(input_tokens),0),'output_tokens',coalesce(sum(output_tokens),0)) FROM model_calls WHERE model_id='%s';`, model))
	if err != nil {
		return err
	}
	var usage struct {
		Calls   int `json:"calls"`
		Unknown int `json:"unknown_usage"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(stats)), &usage) != nil || usage.Calls < 1 || usage.Calls > 128 || usage.Unknown != 0 {
		return errors.New("modern interaction calls lack complete actual provider accounting")
	}
	proof["provider_usage"] = json.RawMessage(stats)
	proof["accepted_at"] = time.Now().UTC()
	return nil
}

func plan11InteractionServiceScript(item plan11InteractionCase, unit string) string {
	return fmt.Sprintf(`set -eu
test -x /usr/local/bin/argus-e2e-status
command -v curl >/dev/null
mkdir %[1]s
printf 'status=broken\n' > %[1]s/status.conf
cat > /etc/systemd/system/%[2]s <<'P11UNIT'
[Unit]
Description=Owned Plan11 current interaction acceptance
[Service]
ExecStart=/usr/local/bin/argus-e2e-status --listen 127.0.0.1:%[3]d --status-file %[1]s/status.conf
Restart=no
KillMode=control-group
P11UNIT
systemctl daemon-reload
systemctl start %[2]s
for attempt in 1 2 3 4 5; do
 code=$(curl -s -o /dev/null -w '%%{http_code}' http://127.0.0.1:%[3]d/ || true)
 if [ "$code" = 503 ]; then printf 'owned_http_503_ready\n'; exit 0; fi
 sleep 1
done
exit 1
`, item.Directory, unit, item.Port)
}
