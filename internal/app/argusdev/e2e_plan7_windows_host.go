package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/remoteexecution"
)

// Foreground enrollment uses the real API and mTLS channel. It never installs a
// service or changes the workstation's accounts, firewall, registry or files
// outside this run's directories and the runner's owned exchange directories.
func (a *App) plan7WindowsEnrollment(ctx context.Context, env *E2EEnvironment) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("Plan7 Windows acceptance requires the explicitly selected Windows host")
	}
	account, err := user.Current()
	if err != nil || account.Uid == "" {
		return "", fmt.Errorf("Windows execution SID unavailable")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	preview, err := client.JSON(ctx, "p7-windows-preview", "enterprise", http.MethodPost, "/enterprise/hosts/actions/preview-create", 201, map[string]any{"name": "Plan7 Windows foreground", "platform": "windows", "architecture": "amd64", "role": "managed_host", "control_path": "direct", "install_method": "manual", "ssh_path": "none", "environment": "development", "labels": map[string]string{"plan": "plan7", "run": env.Options.RunID}, "execution_account": map[string]string{"username": account.Username, "sid": account.Uid}}, enterpriseHeaders(env, "p7-windows-preview"))
	if err != nil {
		return "", err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return "", err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p7-windows-create", ref)
	if err != nil {
		return "", err
	}
	host, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil || uuid.Validate(host) != nil {
		return "", fmt.Errorf("Windows host identity missing")
	}
	oneTime, _ := confirmed["one_time_result"].(map[string]any)
	command := oneTimeCommand(oneTime)
	match := regexp.MustCompile(`X-Argus-Enrollment-Token: ([^']+)`).FindStringSubmatch(command)
	if len(match) != 2 {
		return "", fmt.Errorf("Windows one-time enrollment instruction did not contain its token")
	}
	token := match[1]
	connector, err := a.postgresQuery(ctx, env, "SELECT preallocated_connector_id FROM connector_enrollment_tokens WHERE preallocated_host_id='"+host+"' ORDER BY created_at DESC LIMIT 1;")
	if err != nil {
		return "", err
	}
	connector = strings.TrimSpace(connector)
	if uuid.Validate(connector) != nil {
		return "", fmt.Errorf("Windows Connector allocation missing")
	}
	binary, err := a.buildM3Connector(ctx, env)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(env.WorkDir, "plan7-windows-connector")
	enrollmentEnv := map[string]string{"ARGUS_CONNECTOR_INSTANCE_ID": "plan7-windows-" + env.Options.RunID, "ARGUS_CONNECTOR_ENROLLMENT_TOKEN": token, "ARGUS_CONNECTOR_CA_FILE": env.Endpoints.CAFile, "ARGUS_CONNECTOR_ENROLL_ADDRESS": env.Endpoints.IngressDialAddress, "GLM_API_KEY": ""}
	if err = a.runner.Run(ctx, enrollmentEnv, binary, "enroll", "--connector-id", connector, "--server", env.Endpoints.EnrollServer, "--role", "host", "--name", "plan7-windows-"+env.Options.RunID, "--data-dir", directory); err != nil {
		return "", fmt.Errorf("Windows foreground enrollment failed")
	}
	enrollmentEnv["ARGUS_CONNECTOR_ENROLLMENT_TOKEN"] = ""
	enrollmentEnv["ARGUS_REMOTE_DIRECTORY"] = filepath.Join(env.WorkDir, "plan7-windows-exchange")
	enrollmentEnv["ARGUS_CONNECTOR_DIAL_ADDRESS"] = env.Endpoints.ConnectorDialAddress
	log, err := os.OpenFile(filepath.Join(env.Options.Artifacts, "plan7-windows-connector.log"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer log.Close()
	process, err := a.runner.Start(enrollmentEnv, log, log, binary, "run", "--data-dir", directory)
	if err != nil {
		log.Close()
		return "", err
	}
	env.Processes = append(env.Processes, process)
	if err = a.waitM3ConnectorOnline(ctx, env, connector, 1); err != nil {
		return "", err
	}
	proof, err := a.postgresQuery(ctx, env, "SELECT json_build_object('platform',platform,'username',username,'sid',identity_sid,'binding_source',binding_source,'connector_id',connector_id) FROM remote_execution_identities WHERE resource_id='"+host+"';")
	if err != nil {
		return "", err
	}
	if !strings.Contains(proof, account.Uid) || !strings.Contains(proof, "verified_local_account") {
		return "", fmt.Errorf("Windows enrollment did not persist the explicit SID binding")
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-windows-identity.json"), []byte(proof)); err != nil {
		return "", err
	}
	env.State.Values["p7_windows_host_id"] = host
	return host, nil
}
func (a *App) plan7WindowsControlFlow(ctx context.Context, env *E2EEnvironment, host string) error {
	conversation, err := a.p5Conversation(ctx, env, "Plan7 Windows PowerShell state")
	if err != nil {
		return err
	}
	steps := []p5ToolStep{
		plan7PreviewTool("connect", plan7ToolStep(host, "powershell", "$p7 = [PSCustomObject]@{Count=41}; Set-Location $env:ARGUS_OUTPUT_DIR; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'unicode.txt'),'状态保留',[Text.UTF8Encoding]::new($false)); Write-Output 'ready'")),
		plan7PreviewTool("connect", plan7ToolStep(host, "powershell", "$p7.Count += 1; if ($p7.Count -ne 42) { throw 'PowerShell state was lost' }; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'answer.txt'),[string]$p7.Count); Write-Output $p7.Count")),
	}
	run, err := a.p5StartRun(ctx, env, conversation, "plan7-windows", steps, []string{})
	if err != nil {
		return err
	}
	for n := 1; n <= 2; n++ {
		view, err := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
		if err != nil {
			return err
		}
		if err = a.plan7NoDispatchBeforeApproval(ctx, env, view); err != nil {
			return err
		}
		if err = a.plan7Approve(ctx, env, view); err != nil {
			return err
		}
		result, err := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool {
			return v.ID == view.ID && v.Status != "ready" && v.Status != "running" && v.Status != "awaiting_confirmation"
		})
		if err != nil {
			return err
		}
		if err = a.plan7CompletedEvidence(ctx, env, conversation, fmt.Sprintf("windows-%d", n), result); err != nil {
			return err
		}
	}
	env.State.Values["p7_windows_conversation"] = conversation
	return a.waitRunTerminal(ctx, env, run)
}
