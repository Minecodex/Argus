package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func plan7ToolStep(resource, interpreter, script string) map[string]any {
	return map[string]any{"description": "Plan7 owned resource verification", "target": map[string]any{"resource_type": "host", "resource_id": resource}, "interpreter": interpreter, "script": script, "timeout_seconds": 60}
}
func plan7PreviewTool(kind string, steps ...map[string]any) p5ToolStep {
	input := map[string]any{"title": "Plan7 controlled execution", "description": "Explicit approval of the displayed commands on owned test resources", "steps": steps}
	if kind == "connect" {
		return p5ToolStep{Tool: "connect", Arguments: input}
	}
	return p5ToolStep{Tool: "tool.invoke", Arguments: map[string]any{"category": "workflow", "name": "remote_plan.preview", "arguments": input}}
}
func (a *App) plan7Requests(ctx context.Context, env *E2EEnvironment, conversation string) ([]remoteexecution.View, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	value, err := client.PollJSON(ctx, "p7-executions-"+conversation, "enterprise", "/conversations/"+conversation+"/remote-executions", enterpriseHeaders(env, ""))
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(value)
	var page struct {
		Items []remoteexecution.View `json:"items"`
	}
	if err = json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}
func (a *App) plan7WaitRequest(ctx context.Context, env *E2EEnvironment, conversation string, minimum int, predicate func(remoteexecution.View) bool, expectedRun ...string) (remoteexecution.View, error) {
	wait := 3 * time.Minute
	if len(expectedRun) > 0 {
		// A model-generated preview uses the same bounded budget as the other
		// real-model phases. Command timeouts still come from the approved spec.
		wait = 10 * time.Minute
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		views, err := a.plan7Requests(ctx, env, conversation)
		if err != nil {
			return remoteexecution.View{}, err
		}
		if len(views) >= minimum {
			for _, v := range views {
				if predicate(v) {
					return v, nil
				}
			}
		}
		if len(expectedRun) > 0 {
			client, err := scenarioHTTP(env)
			if err != nil {
				return remoteexecution.View{}, err
			}
			run, err := client.PollJSON(ctx, "p7-remote-run-state", "enterprise", "/runs/"+expectedRun[0], enterpriseHeaders(env, ""))
			if err != nil {
				return remoteexecution.View{}, err
			}
			switch run["status"] {
			case "failed", "cancelled", "timed_out", "succeeded":
				return remoteexecution.View{}, fmt.Errorf("Plan7 run ended before the expected remote state: %v/%v/%v", run["status"], run["error_code"], run["stop_reason"])
			}
		}
		select {
		case <-ctx.Done():
			return remoteexecution.View{}, ctx.Err()
		case <-deadline.C:
			return remoteexecution.View{}, fmt.Errorf("Plan7 remote request did not reach expected state")
		case <-tick.C:
		}
	}
}
func (a *App) plan7Approve(ctx context.Context, env *E2EEnvironment, view remoteexecution.View) error {
	if view.Status != "awaiting_confirmation" || view.ContentHash != remoteexecution.Digest(view.Spec) {
		return fmt.Errorf("Plan7 approval does not match authoritative preview")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	_, err = client.JSON(ctx, "p7-confirm-"+view.ID.String(), "enterprise", http.MethodPost, "/enterprise/pending-actions/"+view.ActionRef+"/confirm", 200, nil, enterpriseHeaders(env, "p7-confirm-"+view.ID.String()))
	return err
}
func (a *App) plan7NoDispatchBeforeApproval(ctx context.Context, env *E2EEnvironment, view remoteexecution.View) error {
	value, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM remote_execution_steps WHERE request_id='"+view.ID.String()+"' AND (command_id<>'' OR status<>'pending');")
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) != "0" {
		return fmt.Errorf("Plan7 dispatched before confirmation")
	}
	return nil
}
func (a *App) plan7CompletedEvidence(ctx context.Context, env *E2EEnvironment, conversation, label string, view remoteexecution.View) error {
	if view.Status != "succeeded" {
		return fmt.Errorf("Plan7 %s finished as %s/%s", label, view.Status, view.ErrorCode)
	}
	for _, result := range view.Results {
		if result.Status != "succeeded" || !result.Terminated || !result.OutputsCollected {
			return fmt.Errorf("Plan7 %s lacks termination or file receipt", label)
		}
	}
	for _, step := range view.Spec.Steps {
		if step.Identity.ID == uuid.Nil || step.SessionID == uuid.Nil || step.Generation == uuid.Nil {
			return fmt.Errorf("Plan7 missing identity/session binding")
		}
	}
	backend := "m4e2e_model_endpoint_fixture"
	if env.State.Values["p6_backend_build"] == "production" {
		backend = "production"
	}
	raw, err := json.MarshalIndent(map[string]any{"scope": "real API, Action worker and Connector execution; deterministic model choices", "backend_build": backend, "conversation_id": conversation, "view": view}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-"+label+".json"), raw)
}

// Exercises two separate confirmations, then a fixed plan, in one live chat.
// Shared shell state and Workspace import are checked through real tool calls.
func (a *App) plan7LinuxControlFlow(ctx context.Context, env *E2EEnvironment, host string) error {
	conversation, err := a.p5Conversation(ctx, env, "Plan7 shared Linux shell and files")
	if err != nil {
		return err
	}
	steps := []p5ToolStep{
		{Tool: "write", Arguments: map[string]any{"path": "remote/input/message.txt", "content": "plan7 frozen bytes\n"}},
		plan7PreviewTool("connect", plan7ToolStep(host, "bash", "p7_value=41; cat \"$ARGUS_INPUT_DIR/message.txt\" > \"$ARGUS_OUTPUT_DIR/copy.txt\"; printf 'ready\\n'")),
		{Tool: "bash", Arguments: map[string]any{"command": "find results -type f -name copy.txt -exec cat {} \\;"}},
		plan7PreviewTool("connect", plan7ToolStep(host, "bash", "p7_value=$((p7_value+1)); test \"$p7_value\" = 42; printf '%s' \"$p7_value\" > \"$ARGUS_OUTPUT_DIR/answer.txt\"")),
		{Tool: "tool.search", Arguments: map[string]any{"category": "workflow"}},
		{Tool: "tool.describe", Arguments: map[string]any{"category": "workflow", "name": "remote_plan.preview"}},
		plan7PreviewTool("plan", plan7ToolStep(host, "bash", "p7_value=$((p7_value+1)); test \"$p7_value\" = 43"), plan7ToolStep(host, "bash", "p7_value=$((p7_value+1)); test \"$p7_value\" = 44; printf '%s' \"$p7_value\" > \"$ARGUS_OUTPUT_DIR/final.txt\"")),
		{Tool: "bash", Arguments: map[string]any{"command": "find results -type f -name final.txt -exec grep -q 44 {} \\;"}},
	}
	run, err := a.p5StartRun(ctx, env, conversation, "plan7-linux-control", steps, []string{})
	if err != nil {
		return err
	}
	var session, generation uuid.UUID
	for n := 1; n <= 3; n++ {
		preview, err := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
		if err != nil {
			return err
		}
		if err = a.plan7NoDispatchBeforeApproval(ctx, env, preview); err != nil {
			return err
		}
		if n == 1 {
			session = preview.Spec.Steps[0].SessionID
			generation = preview.Spec.Steps[0].Generation
		} else if preview.Spec.Steps[0].SessionID != session || preview.Spec.Steps[0].Generation != generation {
			return fmt.Errorf("Plan7 changed shell across approved batches")
		}
		if err = a.plan7Approve(ctx, env, preview); err != nil {
			return err
		}
		finished, err := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool {
			return v.ID == preview.ID && v.Status != "ready" && v.Status != "running" && v.Status != "awaiting_confirmation"
		})
		if err != nil {
			return err
		}
		if err = a.plan7CompletedEvidence(ctx, env, conversation, fmt.Sprintf("linux-control-%d", n), finished); err != nil {
			return err
		}
	}
	if err = a.waitRunTerminal(ctx, env, run); err != nil {
		return err
	}
	proof, err := a.postgresQuery(ctx, env, "SELECT json_build_object('remote_batches',(SELECT count(*) FROM connector_commands WHERE command_type='remote_execution_batch' AND payload->>'requestId' IN (SELECT id::text FROM remote_execution_requests WHERE run_id='"+run+"')),'workspace_analysis',(SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='bash' AND status='succeeded'),'failures',(SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND status<>'succeeded')); ")
	if err != nil {
		return err
	}
	var facts struct {
		Batches  int `json:"remote_batches"`
		Analysis int `json:"workspace_analysis"`
		Failures int `json:"failures"`
	}
	if json.Unmarshal([]byte(proof), &facts) != nil || facts.Batches != 4 || facts.Analysis != 2 || facts.Failures != 0 {
		return fmt.Errorf("Plan7 Linux control facts incomplete: %s", proof)
	}
	env.State.Values["p7_linux_conversation"] = conversation
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-linux-control-facts.json"), []byte(proof))
}
