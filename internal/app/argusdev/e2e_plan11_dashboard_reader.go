package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/modelmetrics"
)

var plan11DashboardReaderSpec = scenarioReaderSpec{
	Prefix: "p11_dashboard_reader", Alias: "plan11-dashboard-reader", RoleName: "Plan11 dashboard reader",
	DisplayName: "Plan11 dashboard reader", UsernamePrefix: "p11-dash-reader-",
	Permissions: []string{"telemetry.dashboard.read", "host.read", "workspace.use"},
}

func (a *App) plan11ReadOnlyDashboards(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	if h == nil || uuid.Validate(h.HostID) != nil || uuid.Validate(h.DashboardID) != nil {
		return errors.New("read-only dashboard acceptance needs its owned measured Host")
	}
	proof := map[string]any{"passed": false, "scope": "actual read-only portal/API; no paid model task", "reader_function_permissions": plan11DashboardReaderSpec.Permissions, "host_data_grant_only": true, "dashboard_data_grant": false}
	defer func() {
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, _ := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-read-only-dashboards.json"), raw))
	}()
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	root, err := client.JSONArray(ctx, "p11-reader-defaults-admin", "enterprise", http.MethodGet, "/dashboards", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	var dashboardID, dashboardName string
	for _, row := range root {
		if row["name"] == "Host" && row["managed_template_key"] != nil && row["managed_template_key"] != "" {
			dashboardID, _ = row["id"].(string)
			dashboardName, _ = row["name"].(string)
		}
	}
	if uuid.Validate(dashboardID) != nil {
		return errors.New("owned Host default dashboard was not discoverable")
	}
	id, alias, err := a.prepareScenarioReader(ctx, env, plan11DashboardReaderSpec)
	if err != nil {
		return err
	}
	proof["reader_id"], proof["default_dashboard_id"] = id, dashboardID
	path := "/enterprise/data-authorizations/user/" + id
	setGrant := func(call context.Context, remove bool) error {
		state, err := client.JSON(call, "p11-reader-grant-state", "enterprise", http.MethodGet, path+"?resource_type=host", 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		_, err = client.JSON(call, "p11-reader-host-grant", "enterprise", http.MethodPost, path, 204, map[string]any{"resource_type": "host", "resource_ids": []string{h.HostID}, "remove": remove, "expected_version": state["authorization_version"]}, enterpriseHeaders(env, "p11-reader-host-"+uuid.NewString()))
		return err
	}
	granted := false
	defer func() {
		if granted {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			failure = errors.Join(failure, setGrant(cleanup, true))
		}
	}()
	headers := func() map[string]string {
		return map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": env.State.Values[plan11DashboardReaderSpec.Prefix+"_csrf"]}
	}
	queryPath := "/dashboards/" + dashboardID + "/execute"
	query := map[string]any{"resource_ids": []string{h.HostID}}
	if _, err := client.JSON(ctx, "p11-reader-ungranted-query", alias, http.MethodPost, queryPath, 403, query, headers()); err != nil {
		return err
	}
	if err := setGrant(ctx, false); err != nil {
		return err
	}
	granted = true
	if err := a.refreshScenarioReader(ctx, env, plan11DashboardReaderSpec); err != nil {
		return err
	}
	for _, denied := range []string{h.DashboardID, modelmetrics.DashboardID(uuid.MustParse(env.State.Values["enterprise_id"])).String()} {
		if _, err := client.JSON(ctx, "p11-reader-private-dashboard", alias, http.MethodGet, "/dashboards/"+denied, 403, nil, headers()); err != nil {
			return err
		}
	}
	execution, err := client.JSON(ctx, "p11-reader-default-execution", alias, http.MethodPost, queryPath, 200, query, headers())
	if err != nil {
		return err
	}
	rawExecution, err := json.MarshalIndent(redactJSON(execution), "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "plan11-reader-default-execution.json"), rawExecution); err != nil {
		return err
	}
	proof["execution_evidence"] = "plan11-reader-default-execution.json (redacted API response representation)"
	series, err := plan11ReadOnlyDashboardSeries(execution, h.HostID)
	if err != nil {
		return err
	}
	proof["actual_observed_series"] = series
	for _, revoked := range []bool{false, true} {
		if revoked {
			if err := setGrant(ctx, true); err != nil {
				return err
			}
			granted = false
			if err := a.refreshScenarioReader(ctx, env, plan11DashboardReaderSpec); err != nil {
				return err
			}
			if _, err := client.JSON(ctx, "p11-reader-revoked-query", alias, http.MethodPost, queryPath, 403, query, headers()); err != nil {
				return err
			}
		}
		cookies, err := portalBrowserCookies(env, alias)
		if err != nil {
			return err
		}
		if err := a.runPlaywright(ctx, env, "plan11-dashboard-reader-real.spec.ts", map[string]string{
			"ARGUS_PLAN11_DASHBOARD_READER_REAL": "1", "ARGUS_PLAN11_READER_COOKIES": cookies,
			"ARGUS_PLAN11_READER_ID": id, "ARGUS_PLAN11_READER_DEFAULT_ID": dashboardID,
			"ARGUS_PLAN11_READER_DEFAULT_NAME": dashboardName, "ARGUS_PLAN11_READER_PRIVATE_ID": h.DashboardID,
			"ARGUS_PLAN11_READER_REVOKED": fmt.Sprint(revoked), "ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-dashboard-reader-" + fmt.Sprint(revoked),
		}); err != nil {
			return err
		}
	}
	proof["desktop_cases"], proof["grant_revoked_at_end"], proof["extra_model_calls"] = 8, true, 0
	return nil
}
