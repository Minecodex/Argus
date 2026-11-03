package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"
)

// These are business metadata/scope tasks on the same owned measured Host.
// No new production tool or mutation is introduced by this acceptance stage.
func (a *App) plan11DeployedBusinessWorkflows(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	if !plan11BusinessWorkflowPhase(env.Options.Plan8Phase) || h == nil || env.Options.RealModel == nil {
		return errors.New("business workflows require explicit real-model owned-host phase")
	}
	return a.plan11DashboardFiltersAndScope(ctx, env, h, true)
}

func (a *App) plan11DashboardFiltersAndScope(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, withScopes bool) (failure error) {
	if h == nil || env.Options.RealModel == nil {
		return errors.New("dashboard filters require owned Host and real model")
	}
	name := "plan11-dashboard-filters.json"
	planned := []string{"plan11-dashboard-filters-zh", "plan11-dashboard-filters-en"}
	if withScopes {
		name = "plan11-business-workflows.json"
		planned = plan8SelectedModelNames("plan11-workflows")
	}
	proof := map[string]any{"passed": false, "planned_tasks": planned, "whole_plan_complete": false}
	rows := []map[string]any{}
	uiCases := []map[string]string{}
	defer func() {
		proof["passed"], proof["cases"] = failure == nil, rows
		raw, e := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, e, writePrivate(filepath.Join(env.Options.Artifacts, name), raw))
	}()
	emptyHost := uuid.NewString()
	if _, err := a.postgresQuery(ctx, env, "INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES('"+emptyHost+"','"+env.State.Values["enterprise_id"]+"','Plan11 metadata host without sources','metadata-only.invalid',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex')) RETURNING id"); err != nil {
		return err
	}
	proof["metadata_only_host_without_sources"] = emptyHost
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	grantPath := "/enterprise/data-authorizations/user/" + env.State.Values["admin_user_id"]
	grant, err := client.JSON(ctx, "p11-workflow-grant-state", "enterprise", http.MethodGet, grantPath+"?resource_type=host", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p11-workflow-grant-empty-host", "enterprise", http.MethodPost, grantPath, 204, map[string]any{"resource_type": "host", "resource_ids": []string{emptyHost}, "remove": false, "expected_version": grant["authorization_version"]}, enterpriseHeaders(env, "p11-workflow-grant-"+uuid.NewString())); err != nil {
		return err
	}
	if err := a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	for _, locale := range []string{"zh", "en"} {
		name := "plan11-dashboard-filters-" + locale
		prompt := fmt.Sprintf("For the authorized host named %q, discover applicable dashboards with resource type host, signal metrics, integration host_metrics. Explain applicable versus partially applicable and whether this directory measured panel samples. Then check the same host/type/integration with signal logs and report the actual match outcome, without inferring absent telemetry. Do not execute dashboard panels, data queries, remote tools or mutations. Answer in English.", h.HostName)
		if locale == "zh" {
			prompt = fmt.Sprintf("找到授权主机%q，筛选适用于host、metrics信号、host_metrics集成的仪表盘。说明适用/部分适用和目录是否执行了面板采样。再按同一主机/类型/集成筛选logs信号，如实说明匹配结果；不能从无匹配推断没有遥测。不要执行面板/数据查询、远程操作或变更。用中文回答。", h.HostName)
		}
		if locale == "zh" {
			prompt += fmt.Sprintf(" 另外加入已授权但未配置来源的主机ID %s，按相同类型/metrics/集成查询两台主机的适用性，解释实际返回的部分适用；这是元数据对照，不要将未配置来源说成实测无数据。", emptyHost)
		} else {
			prompt += fmt.Sprintf(" Also check the measured host together with authorized metadata-only host ID %s using the same type/metrics/integration filters, and explain the actual partial suitability. The second host has no configured sources; do not claim it was sampled or measured empty.", emptyHost)
		}
		row, e := a.plan11PreparedMetadataTask(ctx, env, name, prompt, nil, plan11DashboardFilterTraceAllowed, func(ctx context.Context, _ string, sample *p5BenchmarkSample, row map[string]any) error {
			raw, e := a.postgresQuery(ctx, env, plan11InteractionTraceSQL(sample.RunID))
			if e != nil {
				return e
			}
			return plan11VerifyDashboardFilterCalls(raw, h.HostID, row, emptyHost)
		})
		rows = append(rows, row)
		if evaluated, _ := row["actual_model_evaluated"].(bool); evaluated && row["hard_failure"] != true {
			conversation, _ := row["conversation_id"].(string)
			uiCases = append(uiCases, map[string]string{"name": name, "conversation_id": conversation})
		}
		if e != nil {
			failure = errors.Join(failure, e)
			if errors.Is(e, errPlan11NavigationBoundary) || ctx.Err() != nil {
				return failure
			}
		}
	}
	if len(uiCases) == 2 {
		cookies, e := existingPortalBrowserCookies(env)
		if e != nil {
			return errors.Join(failure, e)
		}
		encoded, _ := json.Marshal(uiCases)
		failure = errors.Join(failure, a.runPlaywright(ctx, env, "plan11-dashboard-discovery-real.spec.ts", map[string]string{"ARGUS_PLAN11_DASHBOARD_DISCOVERY_REAL": "1", "ARGUS_PLAN11_DISCOVERY_COOKIES": cookies, "ARGUS_PLAN11_DISCOVERY_CASES": string(encoded), "ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-dashboard-discovery"}))
	}
	if withScopes {
		failure = errors.Join(failure, a.plan11TaskScopeCases(ctx, env, h))
	}
	return failure
}

func plan11DashboardFilterTraceAllowed(raw string) error {
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("invalid dashboard filter trace")
	}
	for _, call := range calls {
		if call.Tool == "tool.search" || call.Tool == "tool.describe" {
			continue
		}
		allowed := call.Tool == "tool.invoke" && ((call.Input.Category == "observe" && (call.Input.Name == "resources.list" || call.Input.Name == "capabilities.get")) || (call.Input.Category == "dashboard" && call.Input.Name == "list") || (call.Input.Category == "host" && (call.Input.Name == "list" || call.Input.Name == "get")))
		if !allowed {
			return errPlan11NavigationBoundary
		}
	}
	return nil
}

func plan11VerifyDashboardFilterCalls(raw, host string, proof map[string]any, additionalHost ...string) error {
	var calls []struct {
		Tool   string
		Status string
		Input  struct {
			Category, Name string
			Arguments      struct {
				ResourceTypes         []string `json:"resource_types"`
				Signals, Integrations []string
				Resources             []struct{ Type, ID string }
			}
		}
		Projection struct {
			Summary struct {
				Complete bool
				Items    []struct {
					Suitability string
					Sampling    *bool  `json:"sampling_performed"`
					State       string `json:"data_state"`
				}
			}
		}
	}
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("invalid dashboard filter evidence")
	}
	metrics, logs, partial := false, false, len(additionalHost) == 0
	for _, call := range calls {
		if call.Tool != "tool.invoke" || call.Status != "succeeded" || call.Input.Category != "dashboard" || call.Input.Name != "list" {
			continue
		}
		in := call.Input.Arguments
		if len(additionalHost) > 0 && len(in.Resources) == 2 && plan11HostTypeFilterEquivalent(in.ResourceTypes) && len(in.Signals) == 1 && in.Signals[0] == "metrics" && len(in.Integrations) == 1 && in.Integrations[0] == "host_metrics" {
			wanted := map[string]bool{host: false, additionalHost[0]: false}
			for _, ref := range in.Resources {
				if ref.Type == "host" {
					if _, ok := wanted[ref.ID]; ok {
						wanted[ref.ID] = true
					}
				}
			}
			if wanted[host] && wanted[additionalHost[0]] {
				for _, item := range call.Projection.Summary.Items {
					if item.Suitability == "partial" && item.Sampling != nil && !*item.Sampling && item.State == "not_checked" {
						partial = true
					}
				}
			}
		}
		if len(in.Resources) != 1 || in.Resources[0].Type != "host" || in.Resources[0].ID != host || !plan11HostTypeFilterEquivalent(in.ResourceTypes) || len(in.Integrations) != 1 || in.Integrations[0] != "host_metrics" || len(in.Signals) != 1 {
			continue
		}
		if in.Signals[0] == "metrics" && len(call.Projection.Summary.Items) > 0 {
			for _, item := range call.Projection.Summary.Items {
				if item.Sampling == nil || *item.Sampling || item.State != "not_checked" || (item.Suitability != "applicable" && item.Suitability != "partial") {
					return errors.New("metadata discovery invented sampling/applicability")
				}
			}
			metrics = true
		}
		if in.Signals[0] == "logs" && call.Projection.Summary.Complete && len(call.Projection.Summary.Items) == 0 {
			logs = true
		}
	}
	proof["explicit_metric_filter_found"], proof["complete_logs_no_match"] = metrics, logs
	proof["actual_partial_suitability"] = partial
	if !metrics || !logs || !partial {
		return errors.New("requested explicit dashboard filter/complete no-match business outcome missing")
	}
	return nil
}

// Explicit resources already contain the Host type. Repeating the same type in
// resource_types is optional; a conflicting or broader array is not equivalent.
func plan11HostTypeFilterEquivalent(types []string) bool {
	return len(types) == 0 || len(types) == 1 && types[0] == "host"
}
