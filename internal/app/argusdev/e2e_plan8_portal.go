package argusdev

import (
	"context"
	"encoding/json"
	"path/filepath"
)

func (a *App) plan8DeployedPortalWorkspace(ctx context.Context, env *E2EEnvironment) error {
	// Long integration/model stages may outlive the original browser cookie.
	// Refresh through the actual login/MFA path before exporting the session.
	if err := a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	if err := a.runPlaywright(ctx, env, "plan8-workspace-real.spec.ts", map[string]string{"ARGUS_PLAN8_WORKSPACE_REAL": "1", "ARGUS_PLAN8_WORKSPACE_COOKIES": cookies, "ARGUS_E2E_ARTIFACT_SUFFIX": "plan8-workspace"}); err != nil {
		return err
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual_deployed_application_monitoring_host_kubernetes_dashboard_alert_directories_zh_en_light_dark_three_desktop_widths_with_current_login_mfa_and_existing_serious_critical_axe_gate", "browser_cases": 12, "desktop_widths": []int{1280, 1440, 1920}, "routes_per_case": 6, "writes": false, "artifacts": "playwright-plan8-plan8-workspace"}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-portal-workspace.json"), proof)
}
