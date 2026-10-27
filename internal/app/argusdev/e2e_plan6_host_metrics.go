package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func hostMetricIdentityMatches(labels map[string]any, host, source string) bool {
	if labels["argus_resource_id"] != host {
		return false
	}
	logical, _ := labels["argus_collection_id"].(string)
	physical, _ := labels["argus_source_id"].(string)
	if logical != "" {
		id, err := uuid.Parse(host)
		if err != nil {
			return false
		}
		return logical == builtincollection.StableID("host", id, "host_metrics").String() && (physical == "" || physical == source)
	}
	return physical == source
}

func plan6HostObservedValues(v any, host, source string) ([]float64, error) {
	values := []float64{}
	var walk func(any) error
	walk = func(value any) error {
		switch value := value.(type) {
		case []any:
			for _, x := range value {
				if err := walk(x); err != nil {
					return err
				}
			}
		case map[string]any:
			if labels, ok := value["metric"].(map[string]any); ok {
				if !hostMetricIdentityMatches(labels, host, source) {
					return fmt.Errorf("system metric escaped the selected Host/source")
				}
				pair, ok := value["value"].([]any)
				if ok && len(pair) == 2 {
					n, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
					if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
						return fmt.Errorf("nonfinite system metric")
					}
					values = append(values, n)
				}
			}
			for _, x := range value {
				if err := walk(x); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := walk(v)
	return values, err
}

func (a *App) plan6HostMetric(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, name, expression string) (float64, error) {
	client, _ := scenarioHTTP(env)
	end := time.Now().Add(3 * time.Minute)
	for attempt := 0; ; attempt++ {
		v, err := client.JSON(ctx, fmt.Sprintf("host-metric-%s-%d", name, attempt), "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": expression, "resource_ids": []string{h.HostID}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return 0, err
		}
		values, err := plan6HostObservedValues(v, h.HostID, h.SourceID)
		if err != nil {
			raw, _ := json.MarshalIndent(redactJSON(v), "", "  ")
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "host-metric-"+name+"-unexpected.json"), raw)
			return 0, err
		}
		if len(values) > 0 {
			raw, _ := json.MarshalIndent(v, "", "  ")
			if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-metric-"+name+".json"), raw); err != nil {
				return 0, err
			}
			return values[0], nil
		}
		if time.Now().After(end) {
			return 0, fmt.Errorf("real system metric %s missing", name)
		}
		if err = waitContext(ctx, 2*time.Second); err != nil {
			return 0, err
		}
	}
}

func (a *App) plan6HostMetricsDashboard(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	cpu, err := a.plan6HostMetric(ctx, env, h, "cpu-baseline", h.CPUExpression)
	if err != nil {
		return err
	}
	mem, err := a.plan6HostMetric(ctx, env, h, "memory-baseline", h.MemoryExpression)
	if err != nil {
		return err
	}
	if cpu < 0 || cpu > 100 || mem < 0 || mem > 100 {
		return fmt.Errorf("system metric units not percentage")
	}
	h.BaselineCPU = cpu
	h.TriggerCPU = math.Min(90, cpu+2)
	if h.TriggerCPU <= cpu {
		return fmt.Errorf("Host baseline too busy for a bounded CPU load scenario")
	}
	spec := plan6HostDashboardSpec(h.CPUExpression, h.MemoryExpression)
	if v := dashboard.Validate(spec); !v.Valid {
		return fmt.Errorf("Host dashboard invalid: %v", v.Issues)
	}
	client, _ := scenarioHTTP(env)
	input := planV2DraftRequest(spec)
	input["name"] = "Plan6 Docker Host monitoring"
	input["description"] = "Actual hostmetrics through Bastion Edge Gateway; received samples only"
	input["proposed_bindings"] = []dashboard.Binding{{ResourceType: "host", ResourceID: uuid.MustParse(h.HostID)}}
	draft, err := client.JSON(ctx, "host-dashboard-create", "enterprise", http.MethodPost, "/dashboard-drafts", 201, input, enterpriseHeaders(env, "host-dash-create"))
	if err != nil {
		return err
	}
	id, _ := stringField(draft, "id")
	preview, err := client.JSON(ctx, "host-dashboard-preview", "enterprise", http.MethodPost, "/dashboard-drafts/"+id+"/preview", 201, map[string]any{"expected_version": draft["draft_version"]}, enterpriseHeaders(env, "host-dash-preview"))
	if err != nil {
		return err
	}
	ref, _ := stringField(preview, "action_ref")
	if _, err = a.confirmPendingAction(ctx, env, "host-dash-publish", ref); err != nil {
		return err
	}
	stored, err := client.JSON(ctx, "host-dashboard-published", "enterprise", http.MethodGet, "/dashboard-drafts/"+id, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	h.DashboardID, _ = stringField(stored, "dashboard_id")
	execution, err := client.JSON(ctx, "host-dashboard-execute", "enterprise", http.MethodPost, "/dashboards/"+h.DashboardID+"/execute", 200, map[string]any{"resource_ids": []string{h.HostID}}, enterpriseHeaders(env, "host-dash-execute"))
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(redactJSON(execution), "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-dashboard-execution.json"), raw); err != nil {
		return err
	}
	return plan6HostVerifyDashboard(execution, h.HostID, h.SourceID)
}

func plan6HostVerifyDashboard(execution map[string]any, host, source string) error {
	panels, _ := execution["panels"].([]any)
	if len(panels) != 2 || execution["partial"] == true {
		return fmt.Errorf("Host dashboard did not return both complete panels")
	}
	for _, raw := range panels {
		panel, _ := raw.(map[string]any)
		if panel["status"] != "success" {
			return fmt.Errorf("Host panel %v status=%v", panel["id"], panel["status"])
		}
		targets, _ := panel["targets"].([]any)
		if len(targets) != 1 {
			return fmt.Errorf("Host panel target missing")
		}
		target, _ := targets[0].(map[string]any)
		meta, _ := target["meta"].(map[string]any)
		if target["status"] != "success" || target["result_type"] != "matrix" || meta["partial"] == true {
			return fmt.Errorf("Host panel %v target incomplete: %v", panel["id"], target["code"])
		}
		series, _ := target["data"].([]any)
		if len(series) == 0 {
			return fmt.Errorf("Host panel %v has no real matrix", panel["id"])
		}
		for _, rawSeries := range series {
			row, _ := rawSeries.(map[string]any)
			labels, _ := row["metric"].(map[string]any)
			points, _ := row["values"].([]any)
			if !hostMetricIdentityMatches(labels, host, source) || len(points) == 0 {
				return fmt.Errorf("Host matrix lacks scoped real samples")
			}
			for _, rawPoint := range points {
				pair, _ := rawPoint.([]any)
				if len(pair) != 2 {
					return fmt.Errorf("invalid Host matrix point")
				}
				n, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100 {
					return fmt.Errorf("invalid Host percentage value")
				}
			}
		}
	}
	return nil
}

func plan6HostDashboardSpec(cpu, memory string) dashboard.Spec {
	spec := dashboard.EmptySpec()
	spec.DefaultTimeRange.Seconds = 1800
	for i, p := range []struct{ id, title, expr string }{{"cpu", "CPU usage", cpu}, {"memory", "Memory usage", memory}} {
		spec.Panels = append(spec.Panels, dashboard.Panel{ID: p.id, Title: p.title, Type: "timeseries", Signal: "metrics", AuthoringMode: "dsl", ApplicableResourceTypes: []string{"host"}, SourceBinding: dashboard.SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Unit: "%", Decimals: 2, Legend: true, Layout: dashboard.Rectangle{X: i * 6, Y: 0, W: 6, H: 40, MinW: 3, MinH: 10}, Targets: []dashboard.Target{{ID: "main", Language: queryengine.LanguagePromQL, QueryMode: "range", SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: p.expr}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "fixed", Seconds: 30}, ParameterBindings: []dashboard.ParameterBinding{}}}})
	}
	return spec
}
