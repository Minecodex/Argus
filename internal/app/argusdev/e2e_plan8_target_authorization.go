package argusdev

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func (a *App) preparePlan8TargetReader(ctx context.Context, env *E2EEnvironment) (string, string, error) {
	return a.prepareScenarioReader(ctx, env, plan8TargetReaderSpec)
}

func (a *App) refreshPlan8TargetReader(ctx context.Context, env *E2EEnvironment, alias string) error {
	if alias != plan8TargetReaderSpec.Alias {
		return errors.New("unexpected owned target-reader alias")
	}
	return a.refreshScenarioReader(ctx, env, plan8TargetReaderSpec)
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
	window := query["time_range"].(map[string]any)
	catalog := map[string]any{"scope": map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "monitoring_target", "id": row.Target.ID.String()}}}, "signal": "metrics", "kind": "metrics", "observed_only": true, "from": window["from"], "to": window["to"], "limit": 100}
	if _, err = client.JSON(ctx, "p8-target-reader-denied-catalog", alias, http.MethodPost, "/enterprise/telemetry/query/catalog", 403, catalog, headers); err != nil {
		return nil, err
	}
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
	catalogValue, err := client.JSON(ctx, "p8-target-reader-allowed-catalog", alias, http.MethodPost, "/enterprise/telemetry/query/catalog", 200, catalog, headers)
	if err != nil {
		return nil, err
	}
	metrics, ok := catalogValue["metrics"].([]any)
	if !ok || len(metrics) == 0 {
		return nil, errors.New("resource-only reader could not discover actual scoped metric metadata")
	}
	for _, value := range metrics {
		item, ok := value.(map[string]any)
		if !ok || item["observed"] != true || item["name"] == "" {
			return nil, errors.New("resource reader catalog fabricated or omitted observed metric identity")
		}
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
	if _, err = client.JSON(ctx, "p8-target-reader-revoked-catalog", alias, http.MethodPost, "/enterprise/telemetry/query/catalog", 403, catalog, headers); err != nil {
		return nil, err
	}
	return map[string]any{"passed": true, "reader_id": id, "target_id": row.Target.ID, "collection_id": row.ID, "actual_api_user_login_password_change_and_mfa": true, "explicit_target_grant_required": true, "scoped_metric_samples": len(samples), "observed_catalog_metrics": len(metrics), "reader_permissions": []string{"monitoring.read"}, "dashboard_permission_required": false, "revocation_denies_record_query_and_catalog": true, "revocation_denies_record_and_cached_query": true}, nil
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
