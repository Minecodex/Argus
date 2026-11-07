package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) runM2Scenario(ctx context.Context, env *E2EEnvironment) error {
	httpClient := NewDomainScenarioHTTP(env)
	env.State.HTTP = httpClient
	platformUsername := "platform-admin"
	platformPassword := "N7!qP4@vL9#sT2$x"
	enterpriseUsername := "enterprise-admin"
	enterprisePassword := "R8!kW3@zM6#pQ2$x"

	setupToken, err := env.Kube.SecretValue(ctx, env.SystemNS, env.ReleaseID+"-generated-secrets", "setup-token")
	if err != nil {
		return err
	}
	status, err := httpClient.JSON(ctx, "setup-status", "", http.MethodGet, "/setup/status", http.StatusOK, nil, nil)
	if err != nil {
		return err
	}
	if state, _ := stringField(status, "state"); state != "uninitialized" {
		return fmt.Errorf("new installation setup state is %q", state)
	}
	setupBody := map[string]any{
		"enterprise_name": "Acme Evaluation", "timezone": "Asia/Shanghai",
		"username": platformUsername, "display_name": "Initial Administrator", "password": platformPassword,
	}
	setupHeaders := map[string]string{"Origin": env.PlatformOrigin(), "X-Argus-Setup-Token": setupToken, "Idempotency-Key": "setup-" + env.Options.RunID}
	initialized, err := httpClient.JSON(ctx, "setup-initialize", "", http.MethodPost, "/setup/initialize", http.StatusCreated, setupBody, setupHeaders)
	if err != nil {
		return err
	}
	enterpriseID, err := stringField(initialized, "enterprise_id")
	if err != nil {
		return err
	}
	initialAdminID, err := stringField(initialized, "enterprise_user_id")
	if err != nil {
		return err
	}
	initialPlatformID, err := stringField(initialized, "platform_user_id")
	if err != nil {
		return err
	}
	if initialAdminID == initialPlatformID {
		return fmt.Errorf("setup returned the same identity for both portals")
	}
	setupHeaders["Idempotency-Key"] += "-repeat"
	if _, err = httpClient.JSON(ctx, "setup-locked", "", http.MethodPost, "/setup/initialize", http.StatusConflict, setupBody, setupHeaders); err != nil {
		return err
	}

	initialLogin, err := httpClient.JSON(ctx, "bootstrap-enterprise-login", "bootstrap-enterprise", http.MethodPost, "/enterprise/auth/login", http.StatusOK,
		map[string]any{"username": platformUsername, "password": platformPassword}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	sessionEnterpriseID, err := stringField(initialLogin, "authenticated_session", "session", "enterprise_id")
	if err != nil || sessionEnterpriseID != enterpriseID {
		return fmt.Errorf("initial enterprise administrator session does not match the new enterprise")
	}
	initialCSRF, err := stringField(initialLogin, "authenticated_session", "csrf_token")
	if err != nil {
		return err
	}
	const rotatedBootstrapPassword = "S6!rT5%uH8&zD1"
	if _, err = httpClient.JSON(ctx, "bootstrap-enterprise-change-password", "bootstrap-enterprise", http.MethodPut, "/enterprise/account/password", http.StatusNoContent,
		map[string]any{"current_password": platformPassword, "new_password": rotatedBootstrapPassword, "expected_version": 1},
		map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": initialCSRF}); err != nil {
		return err
	}
	if _, err = httpClient.JSON(ctx, "bootstrap-enterprise-old-password-denied", "bootstrap-enterprise", http.MethodPost, "/enterprise/auth/login", http.StatusUnauthorized,
		map[string]any{"username": platformUsername, "password": platformPassword}, map[string]string{"Origin": env.EnterpriseOrigin()}); err != nil {
		return err
	}
	platformLogin, err := httpClient.JSON(ctx, "platform-login", "platform", http.MethodPost, "/platform/auth/login", http.StatusOK,
		map[string]any{"username": platformUsername, "password": platformPassword}, map[string]string{"Origin": env.PlatformOrigin()})
	if err != nil {
		return err
	}
	platformCSRF, err := stringField(platformLogin, "authenticated_session", "csrf_token")
	if err != nil {
		return err
	}
	platformMFA, err := enrollScenarioMFA(ctx, httpClient, "platform", "platform", env.PlatformOrigin(), platformCSRF, env.Options.RunID)
	if err != nil {
		return err
	}
	platformCSRF, err = completeScenarioMFALogin(ctx, httpClient, "platform", "platform", env.PlatformOrigin(), platformUsername, platformPassword, &platformMFA)
	if err != nil {
		return err
	}
	enterprise, err := httpClient.JSON(ctx, "enterprise-by-uuid", "platform", http.MethodGet, "/platform/enterprises/"+enterpriseID, http.StatusOK, nil,
		map[string]string{"Origin": env.PlatformOrigin()})
	if err != nil {
		return err
	}
	if _, hasCode := enterprise["code"]; enterprise["id"] != enterpriseID || hasCode {
		return fmt.Errorf("enterprise lookup returned an unexpected identity")
	}
	admin, err := httpClient.JSON(ctx, "enterprise-admin-create", "platform", http.MethodPost, "/platform/enterprise-admins", http.StatusCreated,
		map[string]any{"enterprise_id": enterpriseID, "username": enterpriseUsername, "display_name": "Enterprise Admin", "email": "enterprise@example.test"},
		map[string]string{"Origin": env.PlatformOrigin(), "X-CSRF-Token": platformCSRF, "Idempotency-Key": "admin-" + env.Options.RunID})
	if err != nil {
		return err
	}
	temporaryPassword, err := stringField(admin, "temporary_password")
	if err != nil {
		return err
	}
	adminUserID, err := stringField(admin, "user", "id")
	if err != nil {
		return err
	}

	temporaryLogin, err := httpClient.JSON(ctx, "enterprise-temp-login", "enterprise", http.MethodPost, "/enterprise/auth/login", http.StatusOK,
		map[string]any{"username": enterpriseUsername, "password": temporaryPassword}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	challengeID, err := stringField(temporaryLogin, "password_change_challenge", "challenge_id")
	if err != nil {
		return err
	}
	passwordChange, err := httpClient.JSON(ctx, "enterprise-password-change", "enterprise", http.MethodPost, "/enterprise/auth/complete-password-change", http.StatusOK,
		map[string]any{"challenge_id": challengeID, "temporary_password": temporaryPassword, "new_password": enterprisePassword}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	enterpriseCSRF, err := stringField(passwordChange, "csrf_token")
	if err != nil {
		return err
	}
	enterpriseMFA, err := enrollScenarioMFA(ctx, httpClient, "enterprise", "enterprise", env.EnterpriseOrigin(), enterpriseCSRF, env.Options.RunID)
	if err != nil {
		return err
	}

	env.State.Values["platform_username"] = platformUsername
	env.State.Values["platform_password"] = platformPassword
	env.State.Values["platform_mfa_secret"] = platformMFA.Secret
	env.State.Values["platform_mfa_last"] = platformMFA.LastCode
	env.State.Values["platform_recovery_codes"] = strings.Join(platformMFA.RecoveryCodes, ",")
	env.State.Values["enterprise_username"] = enterpriseUsername
	env.State.Values["enterprise_password"] = enterprisePassword
	env.State.Values["enterprise_mfa_secret"] = enterpriseMFA.Secret
	env.State.Values["enterprise_mfa_last"] = enterpriseMFA.LastCode
	env.State.Values["enterprise_recovery_codes"] = strings.Join(enterpriseMFA.RecoveryCodes, ",")
	env.State.Values["enterprise_id"] = enterpriseID
	env.State.Values["admin_user_id"] = adminUserID
	env.State.Values["platform_csrf"] = platformCSRF
	env.State.Values["enterprise_csrf"] = enterpriseCSRF

	if env.Options.Suite != "p4" && !suiteHas(env.Options.Suite, "tls") {
		platformProof := platformMFA.RecoveryCodes[0]
		enterpriseProof := enterpriseMFA.RecoveryCodes[0]
		if err := a.runPlaywright(ctx, env, "e2e/m2(-dashboard-dialogs)?-real.spec.ts", map[string]string{
			"ARGUS_M2_E2E": "1", "ARGUS_M2_PLATFORM_USERNAME": platformUsername, "ARGUS_M2_PLATFORM_PASSWORD": platformPassword,
			"ARGUS_M2_PLATFORM_MFA_CODE": platformProof, "ARGUS_M2_ENTERPRISE_USERNAME": enterpriseUsername, "ARGUS_M2_ENTERPRISE_PASSWORD": enterprisePassword,
			"ARGUS_M2_ENTERPRISE_MFA_CODE": enterpriseProof,
		}); err != nil {
			return err
		}
	}
	env.State.Values["platform_recovery_codes"] = strings.Join(platformMFA.RecoveryCodes[1:], ",")
	env.State.Values["enterprise_recovery_codes"] = strings.Join(enterpriseMFA.RecoveryCodes[1:], ",")
	if len(suiteDependencies[env.Options.Suite]) > 1 {
		// Browser logins can consume newer codes after enrollment. Continue
		// from the state synchronized by runPlaywright, not the old enrollment.
		code, err := waitForNextTOTP(enterpriseMFA.Secret, env.State.Values["enterprise_mfa_last"])
		if err != nil {
			return err
		}
		env.State.Values["enterprise_mfa_last"] = code
		_, err = httpClient.JSON(ctx, "enterprise-step-up", "enterprise", http.MethodPost, "/enterprise/auth/step-up", http.StatusOK,
			map[string]any{"code": code}, map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": enterpriseCSRF})
		if err != nil {
			return err
		}
	}
	return nil
}

type scenarioMFA struct {
	Secret        string
	LastCode      string
	RecoveryCodes []string
}

func enrollScenarioMFA(ctx context.Context, client *ScenarioHTTP, audience, clientName, origin, csrf, runID string) (scenarioMFA, error) {
	enrollment, err := client.JSON(ctx, audience+"-mfa-enroll", clientName, http.MethodPost, "/"+audience+"/account/mfa/totp/enroll", http.StatusCreated, nil,
		map[string]string{"Origin": origin, "X-CSRF-Token": csrf, "Idempotency-Key": audience + "-mfa-" + runID})
	if err != nil {
		return scenarioMFA{}, err
	}
	enrollmentID, err := stringField(enrollment, "enrollment_id")
	if err != nil {
		return scenarioMFA{}, err
	}
	secret, err := stringField(enrollment, "secret")
	if err != nil {
		return scenarioMFA{}, err
	}
	code, err := generateTOTP(secret, time.Now())
	if err != nil {
		return scenarioMFA{}, err
	}
	verified, err := client.JSON(ctx, audience+"-mfa-verify", clientName, http.MethodPost, "/"+audience+"/account/mfa/totp/verify", http.StatusOK,
		map[string]any{"enrollment_id": enrollmentID, "code": code}, map[string]string{"Origin": origin, "X-CSRF-Token": csrf})
	if err != nil {
		return scenarioMFA{}, err
	}
	items, ok := verified["codes"].([]any)
	if !ok || len(items) != 10 {
		return scenarioMFA{}, fmt.Errorf("%s MFA did not return 10 recovery codes", audience)
	}
	codes := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return scenarioMFA{}, fmt.Errorf("%s MFA recovery code is invalid", audience)
		}
		codes = append(codes, value)
	}
	return scenarioMFA{Secret: secret, LastCode: code, RecoveryCodes: codes}, nil
}

func completeScenarioMFALogin(
	ctx context.Context,
	client *ScenarioHTTP,
	audience, clientName, origin, username, password string,
	mfa *scenarioMFA,
) (string, error) {
	client.Reset(clientName)
	login, err := client.JSON(ctx, audience+"-mfa-login", clientName, http.MethodPost, "/"+audience+"/auth/login", http.StatusOK,
		map[string]any{"username": username, "password": password}, map[string]string{"Origin": origin})
	if err != nil {
		return "", err
	}
	challenge, err := stringField(login, "mfa_challenge", "challenge_id")
	if err != nil {
		return "", err
	}
	code, err := waitForNextTOTP(mfa.Secret, mfa.LastCode)
	if err != nil {
		return "", err
	}
	completed, err := client.JSON(ctx, audience+"-mfa-complete", clientName, http.MethodPost, "/"+audience+"/auth/mfa/complete", http.StatusOK,
		map[string]any{"challenge_id": challenge, "code": code}, map[string]string{"Origin": origin})
	if err != nil {
		return "", err
	}
	csrf, err := stringField(completed, "csrf_token")
	if err != nil {
		return "", err
	}
	mfa.LastCode = code
	return csrf, nil
}
