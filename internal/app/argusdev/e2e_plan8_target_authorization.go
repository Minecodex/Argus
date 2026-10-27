package argusdev

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

// This account is created, changes its password and enrolls MFA through the
// actual APIs. No inserted password hash, synthetic cookie or SQL grant is used.
func (a *App) preparePlan8TargetReader(ctx context.Context, env *E2EEnvironment) (string, string, error) {
	const alias = "plan8-support-reader"
	if id := env.State.Values["p8_support_reader_id"]; id != "" {
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
	role, err := client.JSON(ctx, "p8-support-reader-role", "enterprise", http.MethodPost, "/enterprise/roles", 201, map[string]any{"name": "Plan8 dedicated target reader", "permissions": []string{"monitoring.read"}}, enterpriseHeaders(env, "p8-support-reader-role"))
	if err != nil {
		return "", "", err
	}
	roleID, err := stringField(role, "id")
	if err != nil {
		return "", "", err
	}
	username := "p8-reader-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	created, err := client.JSON(ctx, "p8-support-reader-user", "enterprise", http.MethodPost, "/enterprise/users", 201, map[string]any{"username": username, "display_name": "Plan8 target scope reader", "department_id": department, "role_ids": []string{roleID}}, enterpriseHeaders(env, "p8-support-reader-user"))
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
	login, err := client.JSON(ctx, "p8-support-reader-login", alias, http.MethodPost, "/enterprise/auth/login", 200, map[string]any{"username": username, "password": temporary}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return "", "", err
	}
	challenge, err := stringField(login, "password_change_challenge", "challenge_id")
	if err != nil {
		return "", "", err
	}
	password := "P8!" + uuid.NewString() + "zQ9"
	completed, err := client.JSON(ctx, "p8-support-reader-password", alias, http.MethodPost, "/enterprise/auth/complete-password-change", 200, map[string]any{"challenge_id": challenge, "temporary_password": temporary, "new_password": password}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return "", "", err
	}
	csrf, err := stringField(completed, "csrf_token")
	if err != nil {
		return "", "", err
	}
	mfa, err := enrollScenarioMFA(ctx, client, "enterprise", alias, env.EnterpriseOrigin(), csrf, env.Options.RunID+"-target-reader")
	if err != nil {
		return "", "", err
	}
	env.State.Values["p8_support_reader_id"], env.State.Values["p8_support_reader_username"], env.State.Values["p8_support_reader_password"], env.State.Values["p8_support_reader_csrf"] = id, username, password, csrf
	env.State.Values["p8_support_reader_mfa_secret"], env.State.Values["p8_support_reader_mfa_last"] = mfa.Secret, mfa.LastCode
	return id, alias, nil
}

func (a *App) refreshPlan8TargetReader(ctx context.Context, env *E2EEnvironment, alias string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	client.Reset(alias)
	login, err := client.JSON(ctx, "p8-target-reader-login-refresh", alias, http.MethodPost, "/enterprise/auth/login", 200, map[string]any{"username": env.State.Values["p8_support_reader_username"], "password": env.State.Values["p8_support_reader_password"]}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	if session, ok := login["authenticated_session"].(map[string]any); ok && login["status"] == "authenticated" {
		csrf, e := stringField(session, "csrf_token")
		if e == nil {
			env.State.Values["p8_support_reader_csrf"] = csrf
		}
		return e
	}
	challenge, err := stringField(login, "mfa_challenge", "challenge_id")
	if err != nil {
		return err
	}
	code, err := waitForNextTOTP(env.State.Values["p8_support_reader_mfa_secret"], env.State.Values["p8_support_reader_mfa_last"])
	if err != nil {
		return err
	}
	completed, err := client.JSON(ctx, "p8-target-reader-mfa-refresh", alias, http.MethodPost, "/enterprise/auth/mfa/complete", 200, map[string]any{"challenge_id": challenge, "code": code}, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	csrf, err := stringField(completed, "csrf_token")
	if err != nil {
		return err
	}
	env.State.Values["p8_support_reader_csrf"], env.State.Values["p8_support_reader_mfa_last"] = csrf, code
	return nil
}

func (a *App) plan8VerifyTargetAuthorization(ctx context.Context, env *E2EEnvironment, row monitoring.Collection, metric string) (proof map[string]any, failure error) {
	id, alias, err := a.preparePlan8TargetReader(ctx, env)
	if err != nil {
		return nil, err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	path := "/enterprise/data-authorizations/user/" + id
	setGrant := func(call context.Context, remove bool) error {
		state, e := client.JSON(call, "p8-target-reader-state", "enterprise", http.MethodGet, path+"?resource_type=monitoring_target", 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		_, e = client.JSON(call, "p8-target-reader-grant", "enterprise", http.MethodPost, path, 204, map[string]any{"resource_type": "monitoring_target", "resource_ids": []string{row.Target.ID.String()}, "remove": remove, "expected_version": state["authorization_version"]}, enterpriseHeaders(env, "p8-target-reader-"+uuid.NewString()))
		return e
	}
	granted := false
	defer func() {
		if granted {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			failure = errors.Join(failure, setGrant(cleanup, true))
		}
	}()
	// Every target starts without a grant for this non-admin subject.
	headers := map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": env.State.Values["p8_support_reader_csrf"]}
	queryPath, query := plan8TargetAuthorizationQuery(row, metric, time.Now())
	if _, err = client.JSON(ctx, "p8-target-reader-denied-record", alias, http.MethodGet, "/monitoring/collections/"+row.ID.String(), 403, nil, headers); err != nil {
		return nil, err
	}
	if _, err = client.JSON(ctx, "p8-target-reader-denied-query", alias, http.MethodPost, queryPath, 403, query, headers); err != nil {
		return nil, err
	}
	if err = setGrant(ctx, false); err != nil {
		return nil, err
	}
	granted = true
	if err = a.refreshPlan8TargetReader(ctx, env, alias); err != nil {
		return nil, err
	}
	headers["X-CSRF-Token"] = env.State.Values["p8_support_reader_csrf"]
	if _, err = client.JSON(ctx, "p8-target-reader-allowed-record", alias, http.MethodGet, "/monitoring/collections/"+row.ID.String(), 200, nil, headers); err != nil {
		return nil, err
	}
	value, err := client.JSON(ctx, "p8-target-reader-allowed-query", alias, http.MethodPost, queryPath, 200, query, headers)
	if err != nil {
		return nil, err
	}
	samples, err := plan8ExtraMetricValues(value, row.Target.ID.String(), row.ID.String())
	if err != nil || len(samples) == 0 {
		return nil, errors.New("authorized target reader did not receive actual scoped observations")
	}
	if err = setGrant(ctx, true); err != nil {
		return nil, err
	}
	granted = false
	if err = a.refreshPlan8TargetReader(ctx, env, alias); err != nil {
		return nil, err
	}
	headers["X-CSRF-Token"] = env.State.Values["p8_support_reader_csrf"]
	if _, err = client.JSON(ctx, "p8-target-reader-revoked-record", alias, http.MethodGet, "/monitoring/collections/"+row.ID.String(), 403, nil, headers); err != nil {
		return nil, err
	}
	if _, err = client.JSON(ctx, "p8-target-reader-revoked-query", alias, http.MethodPost, queryPath, 403, query, headers); err != nil {
		return nil, err
	}
	return map[string]any{"passed": true, "reader_id": id, "target_id": row.Target.ID, "collection_id": row.ID, "actual_api_user_login_password_change_and_mfa": true, "explicit_target_grant_required": true, "scoped_metric_samples": len(samples), "revocation_denies_record_and_cached_query": true}, nil
}

func plan8TargetAuthorizationQuery(row monitoring.Collection, metric string, now time.Time) (string, map[string]any) {
	query := map[string]any{"query": metric, "resource_ids": []string{row.Target.ID.String()}, "time_range": map[string]any{"from": now.Add(-5 * time.Minute).UTC().Format(time.RFC3339Nano), "to": now.Add(time.Minute).UTC().Format(time.RFC3339Nano)}, "budget": telemetryBudget(100)}
	queryPath := "/enterprise/metrics/query"
	if row.LastSampleAt != nil && !row.DesiredEnabled {
		// Historical targets have already stopped. Active targets must keep
		// the current window: their first received sample can precede the
		// particular metric used by the authorization oracle.
		query["time_range"] = map[string]any{"from": row.LastSampleAt.Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano), "to": row.LastSampleAt.Add(time.Second).UTC().Format(time.RFC3339Nano)}
		query["step_seconds"] = float64(15)
		queryPath = "/enterprise/metrics/query_range"
	}
	return queryPath, query
}
