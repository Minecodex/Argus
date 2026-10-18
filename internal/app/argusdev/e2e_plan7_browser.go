package argusdev

import (
	"context"
	"fmt"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"strings"
)

func (a *App) plan7Browser(ctx context.Context, env *E2EEnvironment, host string) error {
	variables := map[string]string{"ARGUS_PLAN7_E2E": "1", "ARGUS_PLAN7_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLAN7_PASSWORD": env.State.Values["enterprise_password"], "ARGUS_PLAN7_LINUX_CHAT": env.State.Values["p7_linux_conversation"], "ARGUS_PLAN7_WINDOWS_CHAT": env.State.Values["p7_windows_conversation"], "ARGUS_PLAN7_MODEL_CHAT": env.State.Values["p7_model_conversation"]}
	if err := a.plan7BrowserStateFixtures(ctx, env, host, variables); err != nil {
		return err
	}
	for _, name := range []string{"confirm", "stop"} {
		conversation, err := a.p5Conversation(ctx, env, "Plan7 browser "+name)
		if err != nil {
			return err
		}
		script := strings.Repeat("# The complete frozen command remains reviewable.\n", 100) + "printf 'browser confirmed\\n'"
		script += "\nprintf 'host,cpu\\nhost-a,42\\n' > \"$ARGUS_OUTPUT_DIR/remote-table.csv\"\nprintf '\\000\\001\\377' > \"$ARGUS_OUTPUT_DIR/remote-binary.txt\""
		if name == "stop" {
			script = "printf 'started\\n'; sleep 30; printf 'must not finish\\n'"
		}
		if _, err = a.p5StartRun(ctx, env, conversation, "plan7-browser-"+name, []p5ToolStep{plan7PreviewTool("connect", plan7ToolStep(host, "bash", script))}, []string{}); err != nil {
			return err
		}
		view, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" })
		if err != nil {
			return err
		}
		if err = a.plan7NoDispatchBeforeApproval(ctx, env, view); err != nil {
			return err
		}
		variables["ARGUS_PLAN7_"+strings.ToUpper(name)+"_CHAT"] = conversation
	}
	if err := a.runPlaywright(ctx, env, "e2e/plan7-(real|states-real).spec.ts", variables); err != nil {
		return fmt.Errorf("Plan7 real browser acceptance: %w", err)
	}
	return nil
}
