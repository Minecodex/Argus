package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"net/http"
	"time"
)

func (a *App) templateHealthKindFacts(ctx context.Context, env *E2EEnvironment, policy, resource string, snapshot json.RawMessage) error {
	for _, health := range []string{"no_data", "error"} {
		id := uuid.NewString()
		now := time.Now().UTC()
		state := alerting.InstanceState{Kind: "health", Name: "Template health episode " + health, PolicyName: "Template real policy", PolicySnapshot: snapshot, Labels: map[string]string{"alert_health": health}, NotificationState: "pending", LastSlot: now, NoiseReasons: []string{}}
		sql := fmt.Sprintf(`INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,last_evaluated_at) VALUES('%s','%s','%s',1,1,'template-kind-%s',ARRAY['%s']::uuid[],'{}',%s,'firing','%s','warning',now(),now());`, id, env.State.Values["enterprise_id"], policy, health, resource, templateSQLJSON(state), health)
		if _, err := a.postgresQuery(ctx, env, sql); err != nil {
			return err
		}
		env.State.Values["template_kind_"+health] = id
	}
	return nil
}

func (a *App) templateQueryShapeSteps(ctx context.Context, env *E2EEnvironment, common map[string]any) ([]p5ToolStep, error) {
	clone := func() map[string]any {
		out := map[string]any{}
		for k, v := range common {
			out[k] = v
		}
		return out
	}
	steps := []p5ToolStep{}
	for _, expression := range []string{"* | stats count() by service_name", "* | stats count() by bin(timestamp, 5m), service_name"} {
		input := clone()
		input["query"] = expression
		steps = append(steps, nativeTemplateStep("log", "query", input))
	}
	client, _ := scenarioHTTP(env)
	sample, err := client.JSON(ctx, "templates-trace-identity", "enterprise", http.MethodPost, "/enterprise/traces/graphql", 200, map[string]any{"query": `query { queryBasicTraces(serviceName:"argus-m7-e2e",pageSize:1) { traces { traceId sourceId resourceId } } }`, "resource_ids": common["resource_ids"], "time_range": telemetryTimeRange(time.Hour), "budget": telemetryBudget(10)}, enterpriseHeaders(env, ""))
	if err != nil {
		return nil, err
	}
	payload, _ := sample["data"].(map[string]any)
	result, _ := payload["queryBasicTraces"].(map[string]any)
	rows, _ := result["traces"].([]any)
	if len(rows) == 0 {
		return nil, fmt.Errorf("missing actual trace identity for template acceptance")
	}
	identity, _ := rows[0].(map[string]any)
	traceID, _ := identity["traceId"].(string)
	sourceID, _ := identity["sourceId"].(string)
	resourceID, _ := identity["resourceId"].(string)
	fields := `traceId rootService rootOperation startTime duration spanCount errorCount completeness spans { spanId parentSpanId serviceName operationName startTime duration status }`
	apm := `basis percentileMethod status coverage { observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount } rows { sourceId resourceId serviceName instanceId instanceName operationName timestamp intervalSeconds samplesPerSecond sampleCount errorCount errorRate durationP95Ms }`
	for _, root := range []string{"queryBasicTraces", "queryBasicTracesByName", "queryTraces", "queryTrace", "queryTraceGraph", "queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED", "queryAPMTopology"} {
		args, selection := "(pageSize:5)", `traces { `+fields+` }`
		switch root {
		case "queryTrace":
			args = fmt.Sprintf(`(traceId:%q,sourceId:%q,resourceId:%q)`, traceID, sourceID, resourceID)
			selection = fields
		case "queryTraceGraph":
			args = fmt.Sprintf(`(traceId:%q,sourceId:%q,resourceId:%q)`, traceID, sourceID, resourceID)
			selection = `traceId completeness spans { spanId parentSpanId serviceName operationName startTime duration status } edges { parentSpanId childSpanId }`
		case "queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED":
			args = "(limit:20)"
			selection = apm
			if root == "queryAPMRED" {
				args = "(bucketSeconds:60,limit:20)"
			}
		case "queryAPMTopology":
			args = "(limit:20)"
			selection = `basis percentileMethod status nodes { id sourceId resourceId serviceName } edges { sourceNodeId targetNodeId sampleCount errorCount durationP95Ms }`
		}
		input := clone()
		input["document"] = "query { " + root + args + " { " + selection + " } }"
		steps = append(steps, nativeTemplateStep("trace", "query", input))
	}
	input := clone()
	input["document"] = `query Combined { services: queryAPMServices(limit:5) { rows { label:serviceName samples:sampleCount } } named: queryBasicTracesByName(pageSize:5) { traces { id:traceId rootService rootOperation duration } } }`
	steps = append(steps, nativeTemplateStep("trace", "query", input))
	return steps, nil
}
