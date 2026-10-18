package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// This focused gate exercises the selected provider through the real composer
// and approval cards. It cannot substitute for the 57-case cross-platform gate.
func (a *App) plan7LinuxChatbox(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	if err := a.plan7TelemetryModelFlow(ctx, env, host); err != nil {
		return err
	}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	fault, err := a.preparePlan7ServiceFault(ctx, env, host)
	if err != nil {
		return err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "linux-chatbox")
	if err != nil {
		return err
	}
	report := filepath.Join(env.Options.Artifacts, "plan7-linux-chatbox.json")
	fixture, _ := json.Marshal(map[string]string{
		"conversation": conversation, "resource_id": host.HostID,
		"directory": fault.Directory, "unit": fault.Unit,
		"container": host.Docker.Host, "owner": host.Docker.Owner,
		"model": env.Options.RealModel.ModelID,
	})
	if err = a.runPlaywright(ctx, env, "e2e/plan7-linux-chatbox-real.spec.ts", map[string]string{
		"ARGUS_PLAN7_LINUX_CHATBOX": "1", "ARGUS_E2E_ARTIFACT_SUFFIX": "linux-chatbox",
		"ARGUS_PLAN7_USERNAME":      env.State.Values["enterprise_username"],
		"ARGUS_PLAN7_PASSWORD":      env.State.Values["enterprise_password"],
		"ARGUS_PLAN7_LINUX_FIXTURE": string(fixture), "ARGUS_PLAN7_FLOW_REPORT": report,
	}); err != nil {
		return err
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		return err
	}
	var result struct {
		Conversation string `json:"conversation"`
		Model        string `json:"model"`
		DiagnosisRun string `json:"diagnosis_run"`
		RepairRun    string `json:"repair_run"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Conversation != conversation || result.Model != env.Options.RealModel.ModelID {
		return fmt.Errorf("Linux Chatbox report does not match this provider and conversation")
	}
	for _, run := range []string{result.DiagnosisRun, result.RepairRun} {
		if uuid.Validate(run) != nil {
			return fmt.Errorf("Linux Chatbox report has no actual Run")
		}
		sample, err := a.p5BenchmarkSample(ctx, env, "linux-chatbox", run)
		if err != nil || sample.RunStatus != "succeeded" || !sample.UsageComplete || sample.ModelCalls < 1 || sample.TotalTokens < 1 {
			return fmt.Errorf("Linux Chatbox Run lacks successful reported and settled model usage: %v", err)
		}
		proof, _ := json.MarshalIndent(sample, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-linux-chatbox-run-"+run+".json"), proof); err != nil {
			return err
		}
	}
	// Validate actual durable ordering, rather than inferring serial execution
	// from the UI's progress labels or the assistant's final message.
	invalid, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT count(*) FROM remote_execution_steps s JOIN remote_execution_requests r ON r.id=s.request_id WHERE r.conversation_id='%s' AND ((r.status='superseded' AND (s.command_id<>'' OR s.started_at IS NOT NULL)) OR (s.sequence>1 AND s.started_at<(SELECT p.completed_at FROM remote_execution_steps p WHERE p.request_id=s.request_id AND p.sequence=s.sequence-1)))`, conversation))
	if err != nil || strings.TrimSpace(invalid) != "0" {
		return fmt.Errorf("Linux Chatbox dispatched an obsolete plan or overlapped steps: %v", err)
	}
	if err = a.verifyPlan7ServiceRecovery(ctx, env, host, fault); err != nil {
		return err
	}
	env.State.Values["p7_model_conversation"] = conversation
	return nil
}
