package argusdev

import "context"

func (a *App) plan11SignalBrowser(ctx context.Context, env *E2EEnvironment, task plan11SignalTask, conversation string) error {
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	return a.runPlaywright(ctx, env, "plan11-signal-results-real.spec.ts", map[string]string{
		"ARGUS_PLAN11_SIGNAL_REAL": "1", "ARGUS_PLAN11_SIGNAL_CONVERSATION": conversation, "ARGUS_PLAN11_SIGNAL_KIND": task.Kind,
		"ARGUS_PLAN11_SIGNAL_COOKIES": cookies, "ARGUS_E2E_ARTIFACT_SUFFIX": task.Name,
	})
}
