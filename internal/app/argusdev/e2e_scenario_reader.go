package argusdev

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type scenarioReaderSpec struct {
	Prefix, Alias, RoleName, DisplayName, UsernamePrefix string
	Permissions                                          []string
}

var plan8TargetReaderSpec = scenarioReaderSpec{
	Prefix: "p8_support_reader", Alias: "plan8-support-reader", RoleName: "Plan8 dedicated target reader", DisplayName: "Plan8 target scope reader", UsernamePrefix: "p8-reader-", Permissions: []string{"monitoring.read"},
}

// This account is created, changes its password and enrolls MFA through the
// actual APIs. No inserted password hash, synthetic cookie or SQL grant is used.
func (a *App) prepareScenarioReader(ctx context.Context, env *E2EEnvironment, spec scenarioReaderSpec) (string, string, error) {
	alias := spec.Alias
	if id := env.State.Values[spec.Prefix+"_id"]; id != "" {
		return id, alias, nil
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", "", err
	}
	enterprise := env.State.Values["enterprise_id"]
	if uuid.Validate(enterprise) != nil {
		return "", "", errors.New("owned enterprise identity missing")
	}
	department, err := a.postgresQuery(ctx, env, "SELECT id FROM departments WHERE enterprise_id='"+enterprise+"' AND is_default=true")
	if err != nil {
		return "", "", err
	}
	department = strings.TrimSpace(department)
	if uuid.Validate(department) != nil {
		return "", "", errors.New("owned department identity missing")
	}
	role, err := client.JSON(ctx, spec.Alias+"-role", "enterprise", http.MethodPost, "/enterprise/roles", 201, map[string]any{"name": spec.RoleName, "permissions": spec.Permissions}, enterpriseHeaders(env, spec.Alias+"-role"))
	if err != nil {
		return "", "", err
	}
	roleID, err := stringField(role, "id")
	if err != nil {
		return "", "", err
	}
	username := spec.UsernamePrefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	created, err := client.JSON(ctx, spec.Alias+"-user", "enterprise", http.MethodPost, "/enterprise/users", 201, map[string]any{"username": username, "display_name": spec.DisplayName, "department_id": department, "role_ids": []string{roleID}}, enterpriseHeaders(env, spec.Alias+"-user"))
	if err != nil {
		return "", "", err
	}
	id, err := stringField(created, "user", "id")
	if err != nil {
		return "", "", err
	}
	temporary, err := stringField(created, "temporary_password")
	if err != nil {
		return "", "", err
	}
	login, err := client.JSON(ctx, spec.Alias+"-login", alias, http.MethodPost, "/enterprise/auth/login", 200, map[string]any{"username": username, "password": temporary}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return "", "", err
	}
	challenge, err := stringField(login, "password_change_challenge", "challenge_id")
	if err != nil {
		return "", "", err
	}
	password := "P8!" + uuid.NewString() + "zQ9"
	completed, err := client.JSON(ctx, spec.Alias+"-password", alias, http.MethodPost, "/enterprise/auth/complete-password-change", 200, map[string]any{"challenge_id": challenge, "temporary_password": temporary, "new_password": password}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return "", "", err
	}
	csrf, err := stringField(completed, "csrf_token")
	if err != nil {
		return "", "", err
	}
	mfa, err := enrollScenarioMFA(ctx, client, "enterprise", alias, env.EnterpriseOrigin(), csrf, env.Options.RunID+"-"+spec.Alias)
	if err != nil {
		return "", "", err
	}
	env.State.Values[spec.Prefix+"_id"], env.State.Values[spec.Prefix+"_username"], env.State.Values[spec.Prefix+"_password"], env.State.Values[spec.Prefix+"_csrf"] = id, username, password, csrf
	env.State.Values[spec.Prefix+"_mfa_secret"], env.State.Values[spec.Prefix+"_mfa_last"] = mfa.Secret, mfa.LastCode
	return id, alias, nil
}

func (a *App) refreshScenarioReader(ctx context.Context, env *E2EEnvironment, spec scenarioReaderSpec) error {
	alias := spec.Alias
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	client.Reset(alias)
	login, err := client.JSON(ctx, spec.Alias+"-login-refresh", alias, http.MethodPost, "/enterprise/auth/login", 200, map[string]any{"username": env.State.Values[spec.Prefix+"_username"], "password": env.State.Values[spec.Prefix+"_password"]}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	if session, ok := login["authenticated_session"].(map[string]any); ok && login["status"] == "authenticated" {
		csrf, e := stringField(session, "csrf_token")
		if e == nil {
			env.State.Values[spec.Prefix+"_csrf"] = csrf
		}
		return e
	}
	challenge, err := stringField(login, "mfa_challenge", "challenge_id")
	if err != nil {
		return err
	}
	code, err := waitForNextTOTP(env.State.Values[spec.Prefix+"_mfa_secret"], env.State.Values[spec.Prefix+"_mfa_last"])
	if err != nil {
		return err
	}
	completed, err := client.JSON(ctx, spec.Alias+"-mfa-refresh", alias, http.MethodPost, "/enterprise/auth/mfa/complete", 200, map[string]any{"challenge_id": challenge, "code": code}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	csrf, err := stringField(completed, "csrf_token")
	if err != nil {
		return err
	}
	env.State.Values[spec.Prefix+"_csrf"], env.State.Values[spec.Prefix+"_mfa_last"] = csrf, code
	return nil
}
