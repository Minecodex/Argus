package argusdev

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
)

// Deterministic PromQL executes in the real query engine. These are explicitly
// synthetic formula observations, not fabricated production monitoring data.
func templateSamplingExpression(kind string) string {
	condition := "floor(vector(time())) % 600 == 1"
	if kind == "gap" {
		condition = "floor(vector(time())) % 600 == 3"
	}
	if kind == "gap-budget" {
		condition = "floor(vector(time())) % 2 == 1"
	}
	return fmt.Sprintf(`label_replace((vector(10000) and on() (floor(vector(time())) %% 600 == 2)) or ((vector(0)/vector(0)) and on() (%s)) or vector(1), "__name__", "template_sampling_%s", "", "")`, condition, kind)
}

func templateSamplingSteps(resource string) []p5ToolStep {
	start := time.Now().UTC().Truncate(10 * time.Minute).Add(-10 * time.Minute)
	steps := []p5ToolStep{}
	for _, kind := range []string{"peak", "gap", "gap-budget"} {
		steps = append(steps, nativeTemplateStep("metric", "query", map[string]any{"resource_ids": []string{resource}, "query": templateSamplingExpression(kind), "from": start.Format(time.RFC3339), "to": start.Add(599 * time.Second).Format(time.RFC3339), "step_seconds": 1, "limit": 1000}))
	}
	return steps
}

func addTemplateSamplingPanels(spec *dashboard.Spec) {
	for i, kind := range []string{"peak", "gap-budget"} {
		panel := spec.Panels[0]
		panel.ID, panel.Title = "sampling-"+kind, "Real metric sampling "+kind
		panel.Layout.Y = (i + 4) * 52
		panel.Targets = append([]dashboard.Target{}, panel.Targets...)
		panel.Targets[0].SourceDefinition = dashboard.Definition{DSL: &dashboard.DSL{Expression: templateSamplingExpression(kind)}}
		panel.Targets[0].RangeStepPolicy = dashboard.StepPolicy{Kind: "fixed", Seconds: 1}
		spec.Panels = append(spec.Panels, panel)
	}
}

func validateTemplateMetricPreview(value any) ([]any, error) {
	proof := []any{}
	var visit func(any) error
	visit = func(value any) error {
		switch item := value.(type) {
		case map[string]any:
			metric, _ := item["metric"].(map[string]any)
			name, _ := metric["__name__"].(string)
			if strings.HasPrefix(name, "template_sampling_") {
				preview, _ := item["argus_sample_preview"].(map[string]any)
				stats, _ := item["argus_statistics"].(map[string]any)
				points, _ := item["values"].([]any)
				if stats["max"] != float64(10000) || len(points) > 200 {
					return fmt.Errorf("invalid metric statistics/point budget: %s points=%d max=%v samples=%v preview=%v", name, len(points), stats["max"], stats["sample_count"], preview["state"])
				}
				gap, high := 0, 0.
				for _, raw := range points {
					pair := raw.([]any)
					n, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
					if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
						gap++
					} else {
						high = math.Max(high, n)
					}
				}
				if strings.HasSuffix(name, "gap-budget") {
					if len(points) != 0 || preview["state"] != "omitted_gap_budget" {
						return fmt.Errorf("metric gaps silently bridged: %s", name)
					}
				} else if high != 10000 || gap == 0 || preview["state"] != "sampled" {
					return fmt.Errorf("metric extrema or gaps lost: %s", name)
				}
				proof = append(proof, map[string]any{"name": name, "preview": preview, "statistics": stats, "plotted_max": high, "missing_markers": gap})
			}
			for _, child := range item {
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range item {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := visit(value)
	return proof, err
}

func (a *App) templateExactMetricPages(ctx context.Context, env *E2EEnvironment, path string, expected int, selection ...map[string]any) error {
	client, _ := scenarioHTTP(env)
	count, cursor := 0, ""
	for {
		parameters := map[string]any{"collection": "items", "index": 0, "field": "values", "limit": 100}
		for _, extra := range selection {
			for key, value := range extra {
				parameters[key] = value
			}
		}
		if cursor != "" {
			parameters["cursor"] = cursor
		}
		page, err := client.JSON(ctx, fmt.Sprintf("templates-exact-metric-%d", count), "enterprise", http.MethodPost, path+"/read", 200, map[string]any{"operation": "result.page", "parameters": parameters}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		data, _ := page["data"].(map[string]any)
		rows, _ := data["items"].([]any)
		count += len(rows)
		cursor, _ = page["next_cursor"].(string)
		if count > expected {
			return fmt.Errorf("extra metric samples")
		}
		if cursor == "" {
			break
		}
	}
	if count != expected {
		return fmt.Errorf("expected %d exact metric samples, got %d", expected, count)
	}
	return nil
}

func (a *App) templateDashboardMetricPages(ctx context.Context, env *E2EEnvironment, path string, detail map[string]any) (int, error) {
	count := 0
	panels, _ := detail["panels"].([]any)
	for _, raw := range panels {
		panel, _ := raw.(map[string]any)
		id, _ := panel["id"].(string)
		if !strings.HasPrefix(id, "sampling-") {
			continue
		}
		targets, _ := panel["targets"].([]any)
		for _, raw := range targets {
			target := raw.(map[string]any)
			rows, _ := target["data"].([]any)
			if len(rows) != 1 {
				return 0, fmt.Errorf("missing frozen metric matrix")
			}
			stats, _ := rows[0].(map[string]any)["argus_statistics"].(map[string]any)
			samples, _ := stats["sample_count"].(float64)
			if samples < 600 {
				return 0, fmt.Errorf("insufficient frozen metric samples")
			}
			if err := a.templateExactMetricPages(ctx, env, path, int(samples), map[string]any{"panel_id": id, "target_id": target["id"]}); err != nil {
				return 0, err
			}
			count += int(samples)
		}
	}
	return count, nil
}
