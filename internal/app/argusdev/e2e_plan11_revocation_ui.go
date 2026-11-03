package argusdev

import (
	"context"

	"github.com/google/uuid"
)

func (a *App) plan11RevocationBrowser(ctx context.Context, env *E2EEnvironment, conversation string, revoked, retained, file uuid.UUID) error {
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	return a.runPlaywright(ctx, env, "plan11-revocation-real.spec.ts", map[string]string{
		"ARGUS_PLAN11_REVOCATION_REAL": "1", "ARGUS_PLAN11_REVOCATION_COOKIES": cookies,
		"ARGUS_PLAN11_REVOCATION_CONVERSATION": conversation, "ARGUS_PLAN11_REVOKED_APPLICATION": revoked.String(),
		"ARGUS_PLAN11_RETAINED_APPLICATION": retained.String(), "ARGUS_PLAN11_REVOKED_FILE": file.String(),
		"ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-revocation",
	})
}

func plan11RevocationBrowserSelected(phase string) bool {
	return phase == "plan11-revocation" || phase == "plan11-access" || phase == "plan11-review" || phase == "plan11"
}
