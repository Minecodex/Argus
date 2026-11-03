package argusdev

import (
	"context"
	"encoding/json"
	"path/filepath"
)

// This reads an actual completed model conversation through its normal portal
// session. Model tokens, MFA secrets and session cookies never enter evidence.
func (a *App) plan11DeployedChatbox(ctx context.Context, env *E2EEnvironment, conversation string, host *plan6HostLoop) error {
	if err := a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	err = a.runPlaywright(ctx, env, "plan11-model-discovery-real.spec.ts", map[string]string{
		"ARGUS_PLAN11_MODEL_REAL": "1", "ARGUS_PLAN11_MODEL_COOKIES": cookies,
		"ARGUS_PLAN11_MODEL_CONVERSATION": conversation, "ARGUS_PLAN11_HOST_ID": host.HostID, "ARGUS_PLAN11_HOST_NAME": host.HostName,
		"ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-discovery",
	})
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(map[string]any{"passed": true, "conversation_id": conversation, "scope": "actual_authorized_chatbox_native_observation_views_original_query_and_report_download", "artifacts": "playwright-plan8-plan11-discovery"}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-actual-chatbox.json"), raw)
}
