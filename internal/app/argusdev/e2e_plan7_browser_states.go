package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func (a *App) plan7BrowserStateFixtures(ctx context.Context, env *E2EEnvironment, host string, variables map[string]string) error {
	windows := env.State.Values["p7_windows_host_id"]
	if windows == "" {
		return fmt.Errorf("browser Windows fixture unavailable")
	}
	for _, fixture := range []struct {
		name    string
		steps   []map[string]any
		approve bool
		state   string
	}{
		{"review", []map[string]any{plan7ToolStep(host, "bash", "printf 'Linux review\\n'"), plan7ToolStep(windows, "powershell", "Write-Output 'Windows review'")}, false, "awaiting_confirmation"},
		{"expired", []map[string]any{plan7ToolStep(host, "bash", "printf 'expired must not execute\\n'")}, false, "expired"},
		{"failure", []map[string]any{plan7ToolStep(host, "bash", "printf 'first succeeded\\n'"), plan7ToolStep(host, "bash", "printf 'expected failure\\n' >&2; false"), plan7ToolStep(host, "bash", "printf 'must not execute\\n'")}, true, "partial_failed"},
		{"recovery", []map[string]any{plan7ToolStep(host, "bash", `kill -KILL "$BASHPID"`)}, true, "paused_for_recovery"},
	} {
		conversation, err := a.p5Conversation(ctx, env, "Plan7 browser "+fixture.name)
		if err != nil {
			return err
		}
		run, err := a.p5StartRun(ctx, env, conversation, "plan7-browser-"+fixture.name, plan7PlanToolSequence(plan7PreviewTool("plan", fixture.steps...)), []string{})
		if err != nil {
			return err
		}
		view, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
		if err != nil {
			return err
		}
		if err = a.plan7NoDispatchBeforeApproval(ctx, env, view); err != nil {
			return err
		}
		if fixture.approve {
			if err = a.plan7Approve(ctx, env, view); err != nil {
				return err
			}
		} else if fixture.name == "expired" {
			if _, err = a.postgresQuery(ctx, env, "UPDATE pending_actions SET expires_at=now()-interval '1 second' WHERE action_ref='"+view.ActionRef+"' AND status='awaiting_confirmation';"); err != nil {
				return err
			}
		}
		final, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool { return v.ID == view.ID && v.Status == fixture.state })
		if err != nil {
			return err
		}
		if fixture.name == "failure" && (len(final.Results) != 3 || final.Results[0].Status != "succeeded" || final.Results[1].Status != "failed" || final.Results[2].Status != "skipped") {
			return fmt.Errorf("browser failure fixture did not preserve serial execution facts")
		}
		if fixture.name == "recovery" {
			if err = a.waitRunTerminal(ctx, env, run); err != nil {
				return err
			}
			tool := plan7PreviewTool("plan", plan7ToolStep(host, "bash", "printf 'recovery approved\\n'"))
			tool.Arguments["arguments"].(map[string]any)["recovery_of_id"] = view.ID.String()
			nextRun, e := a.p5StartRun(ctx, env, conversation, "plan7-browser-recovery-preview", plan7PlanToolSequence(tool), []string{})
			if e != nil {
				return e
			}
			next, e := a.plan7WaitRequest(ctx, env, conversation, 2, func(v remoteexecution.View) bool {
				return v.Spec.RunID.String() == nextRun && v.Status == "awaiting_confirmation"
			}, nextRun)
			if e != nil {
				return e
			}
			if next.Spec.RecoveryOfID == nil || *next.Spec.RecoveryOfID != view.ID || next.Spec.Steps[0].Generation == view.Spec.Steps[0].Generation {
				return fmt.Errorf("browser recovery did not bind a fresh interpreter and original request")
			}
			if e = a.plan7NoDispatchBeforeApproval(ctx, env, next); e != nil {
				return e
			}
		}
		variables["ARGUS_PLAN7_"+strings.ToUpper(fixture.name)+"_CHAT"] = conversation
		proof, _ := json.MarshalIndent(final, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-browser-"+fixture.name+".json"), proof); err != nil {
			return err
		}
	}
	return nil
}
