package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Two actual user identities, independent personal drafts and object grants.
// No SQL shortcut creates a password, session, grant or published revision.
func (a *App) preparePlanV2Editor(ctx context.Context, env *E2EEnvironment) error {
	client, _ := scenarioHTTP(env)
	department, err := a.postgresQuery(ctx, env, "SELECT id FROM departments WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' AND is_default=true;")
	if err != nil {
		return err
	}
	user, err := client.JSON(ctx, "p2-editor-create", "enterprise", http.MethodPost, "/enterprise/users", 201, map[string]any{"username": "p2-editor", "display_name": "PlanV2 Editor", "department_id": strings.TrimSpace(department), "role_ids": []string{env.State.Values["m4_resource_admin_role_id"]}}, enterpriseHeaders(env, "p2-editor-create"))
	if err != nil {
		return err
	}
	id, err := stringField(user, "user", "id")
	if err != nil {
		return err
	}
	temporary, err := stringField(user, "temporary_password")
	if err != nil {
		return err
	}
	login, err := client.JSON(ctx, "p2-editor-login", "p2-editor", http.MethodPost, "/enterprise/auth/login", 200, map[string]any{"username": "p2-editor", "password": temporary}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	challenge, err := stringField(login, "password_change_challenge", "challenge_id")
	if err != nil {
		return err
	}
	password := "P2!" + uuid.NewString() + "zQ9"
	changed, err := client.JSON(ctx, "p2-editor-password", "p2-editor", http.MethodPost, "/enterprise/auth/complete-password-change", 200, map[string]any{"challenge_id": challenge, "temporary_password": temporary, "new_password": password}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	csrf, err := stringField(changed, "csrf_token")
	if err != nil {
		return err
	}
	mfa, err := enrollScenarioMFA(ctx, client, "enterprise", "p2-editor", env.EnterpriseOrigin(), csrf, env.Options.RunID+"-editor")
	if err != nil {
		return err
	}
	env.State.Values["p2_editor_id"], env.State.Values["p2_editor_username"], env.State.Values["p2_editor_password"] = id, "p2-editor", password
	env.State.Values["p2_editor_mfa_secret"], env.State.Values["p2_editor_mfa_last"] = mfa.Secret, mfa.LastCode
	spec, err := planV2ThreeSignals()
	if err != nil {
		return err
	}
	spec.Panels = spec.Panels[:1]
	board, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 collaboration", spec)
	if err != nil {
		return err
	}
	env.State.Values["p2_collaboration_dashboard_id"] = board
	path := "/enterprise/data-authorizations/user/" + id
	state, err := client.JSON(ctx, "p2-editor-grants", "enterprise", http.MethodGet, path+"?resource_type=dashboard", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p2-editor-dashboard-grant", "enterprise", http.MethodPost, path, 204, map[string]any{"resource_type": "dashboard", "resource_ids": []string{board}, "remove": false, "expected_version": state["authorization_version"]}, enterpriseHeaders(env, "p2-editor-dashboard-grant")); err != nil {
		return err
	}
	if env.State.Values["p2_editor_id"] == env.State.Values["admin_user_id"] {
		return fmt.Errorf("PlanV2 editors must be different subjects")
	}
	return nil
}
