package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kakj-go/Argus/internal/dashboard"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const templateAPMFields = `basis percentileMethod status windowStart windowEnd coverage { observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount } rows { sourceId resourceId serviceName instanceId instanceName operationName timestamp intervalSeconds samplesPerSecond sampleCount errorCount errorRate durationP95Ms }`
const templateTopologyFields = `basis percentileMethod status windowStart windowEnd nodes { id sourceId resourceId serviceName } edges { sourceNodeId targetNodeId sampleCount errorCount durationP95Ms }`

func validateTemplateAPMLimitView(view map[string]any) error {
	key, _ := view["key"].(string)
	if !strings.HasPrefix(key, "limit_") {
		return nil
	}
	parts := strings.SplitN(key, "_", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid native APM fixture alias")
	}
	limited, present := view["limited"]
	if parts[1] == "unknown" && !present {
		return nil
	}
	if value, ok := limited.(bool); ok && parts[1] != "unknown" && value == (parts[1] == "true") {
		return nil
	}
	return fmt.Errorf("native APM fixture %s expected limited=%s, got %v", key, parts[1], limited)
}

func (a *App) templateREDObservations(ctx context.Context, env *E2EEnvironment) error {
	out, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--template-red-fixture", "--resource-id="+env.State.Values["m3_cluster_id"])
	if err != nil {
		return err
	}
	var fixture map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &fixture) != nil || fixture["observations"] != float64(1200) {
		return fmt.Errorf("invalid RED OTLP fixture receipt")
	}
	raw, _ := json.Marshal(fixture)
	env.State.Values["template_red_fixture"] = string(raw)
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "tool-template-v2-red-otlp.json"), raw); err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		result, e := client.JSON(ctx, "templates-red-arrival", "enterprise", http.MethodPost, "/enterprise/traces/graphql", 200, map[string]any{"query": `query { queryAPMRED(serviceName:"argus-template-red-a",bucketSeconds:1,limit:1000) { limited rows { timestamp sampleCount } } }`, "resource_ids": []string{env.State.Values["m3_cluster_id"]}, "time_range": telemetryTimeRange(time.Hour), "budget": telemetryBudget(2000)}, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		payload, _ := result["data"].(map[string]any)
		view, _ := payload["queryAPMRED"].(map[string]any)
		rows, _ := view["rows"].([]any)
		if len(rows) == 600 && view["limited"] == false {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("RED samples did not traverse real OTLP, Kafka, ClickHouse and Query")
}

func templateCompletenessSteps(common map[string]any) []p5ToolStep {
	steps := []p5ToolStep{}
	for _, root := range []string{"queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED", "queryAPMTopology"} {
		for _, state := range []string{"true", "false", "unknown"} {
			args, fields := "limit:1", templateAPMFields
			if state != "true" {
				args = `serviceName:"argus-template-red-a",limit:10`
			}
			if root == "queryAPMRED" {
				bucket := 1
				if state != "true" {
					bucket = 86400
				}
				args += fmt.Sprintf(",bucketSeconds:%d", bucket)
			}
			if root == "queryAPMTopology" {
				fields = templateTopologyFields
			}
			if state != "unknown" {
				fields += " limited"
			}
			input := map[string]any{}
			for key, value := range common {
				input[key] = value
			}
			input["document"] = fmt.Sprintf("query { limit_%s_%s:%s(%s) { %s } }", state, root, root, args, fields)
			input["limit"] = float64(3000)
			steps = append(steps, nativeTemplateStep("trace", "query", input))
		}
	}
	input := map[string]any{}
	for key, value := range common {
		input[key] = value
	}
	input["limit"] = float64(3000)
	input["document"] = `query { template_red_a:queryAPMRED(serviceName:"argus-template-red-a",bucketSeconds:1,limit:1000) { ` + templateAPMFields + ` limited } template_red_b:queryAPMRED(serviceName:"argus-template-red-b",bucketSeconds:1,limit:1000) { ` + templateAPMFields + ` limited } }`
	return append(steps, nativeTemplateStep("trace", "query", input))
}

func addTemplateREDPanel(spec *dashboard.Spec) {
	panel := spec.Panels[2]
	panel.ID, panel.Title, panel.Type = "template-red", "Real APM whole window", "apm_red"
	panel.Layout.Y = 156
	panel.Targets = append([]dashboard.Target{}, panel.Targets...)
	panel.Targets[0].SourceDefinition = dashboard.Definition{DSL: &dashboard.DSL{Expression: `query { queryAPMRED(serviceName:"argus-template-red-a",bucketSeconds:1,limit:1000) { ` + templateAPMFields + ` limited } }`}}
	spec.Panels = append(spec.Panels, panel)
}

func (a *App) templateExactAPMPages(ctx context.Context, env *E2EEnvironment, path string, index int, expected int, selection ...map[string]any) error {
	client, _ := scenarioHTTP(env)
	count, cursor := 0, ""
	for {
		parameters := map[string]any{"collection": "result", "index": index, "field": "rows", "limit": 100}
		for _, extra := range selection {
			for key, value := range extra {
				parameters[key] = value
			}
		}
		if cursor != "" {
			parameters["cursor"] = cursor
		}
		page, err := client.JSON(ctx, fmt.Sprintf("templates-exact-apm-%d-%d", index, count), "enterprise", http.MethodPost, path+"/read", 200, map[string]any{"operation": "result.page", "parameters": parameters}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		data, _ := page["data"].(map[string]any)
		rows, _ := data["items"].([]any)
		count += len(rows)
		if count > expected {
			return fmt.Errorf("unexpected extra frozen APM rows")
		}
		cursor, _ = page["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if count != expected {
		return fmt.Errorf("expected %d exact frozen APM rows, got %d", expected, count)
	}
	return nil
}

func templatePreview(value any) map[string]any {
	switch item := value.(type) {
	case map[string]any:
		if preview, ok := item["chart_preview"].(map[string]any); ok {
			return preview
		}
		for _, child := range item {
			if found := templatePreview(child); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range item {
			if found := templatePreview(child); found != nil {
				return found
			}
		}
	}
	return nil
}

func (a *App) templateDashboardAPMProof(ctx context.Context, env *E2EEnvironment, path string, detail map[string]any) ([]any, error) {
	var receipt map[string]any
	_ = json.Unmarshal([]byte(env.State.Values["template_red_fixture"]), &receipt)
	proof := []any{}
	panels, _ := detail["panels"].([]any)
	for _, raw := range panels {
		panel, _ := raw.(map[string]any)
		if panel["type"] != "apm_red" {
			continue
		}
		targets, _ := panel["targets"].([]any)
		for _, raw := range targets {
			target, _ := raw.(map[string]any)
			preview := templatePreview(target["data"])
			if preview == nil || preview["returned_rows"] != float64(600) {
				return nil, fmt.Errorf("missing full-window frozen dashboard preview")
			}
			series, _ := preview["series"].([]any)
			if len(series) != 1 {
				return nil, fmt.Errorf("unexpected frozen dashboard series")
			}
			group, _ := series[0].(map[string]any)
			points, _ := group["points"].([]any)
			if len(points) == 0 || points[0].(map[string]any)["timestamp"] != receipt["first_sample_at"] || points[len(points)-1].(map[string]any)["timestamp"] != receipt["last_sample_at"] {
				return nil, fmt.Errorf("frozen dashboard preview clipped endpoints")
			}
			spike, gap := false, false
			for _, raw := range points {
				point, _ := raw.(map[string]any)
				spike = spike || point["durationP95Ms"] == float64(900)
				gap = gap || point["sampleCount"] == float64(0) && point["samplesPerSecond"] == nil && point["durationP95Ms"] == nil
			}
			if !spike || !gap {
				return nil, fmt.Errorf("frozen dashboard preview lost the actual P95 spike or no-request gap")
			}
			if err := a.templateExactAPMPages(ctx, env, path, 0, 600, map[string]any{"panel_id": panel["id"], "target_id": target["id"]}); err != nil {
				return nil, err
			}
			proof = append(proof, map[string]any{"panel_id": panel["id"], "target_id": target["id"], "preview": preview, "exact_rows": 600})
		}
	}
	return proof, nil
}
