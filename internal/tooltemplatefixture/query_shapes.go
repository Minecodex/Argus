package tooltemplatefixture

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func queryShapeScenarios() map[string]Fixture {
	out := map[string]Fixture{}
	for _, kind := range []string{"no_data", "error"} {
		base := alertFixture("alert-event-detail", "event")
		raw, _ := json.Marshal(base["instance"])
		var instance alerting.Instance
		_ = json.Unmarshal(raw, &instance)
		instance.Health = kind
		instance.State.Kind = "health"
		instance.State.Name = "Query health: " + kind
		instance.State.Members = nil
		instance.State.Value = nil
		instance.State.Threshold = 0
		instance.State.RecoveryThreshold = 0
		instance.State.Unit = ""
		instance.State.Metric = ""
		instance.State.RuleID = uuid.Nil
		instance.State.LastSuccessfulAt = nil
		for _, view := range []struct{ name, tool string }{{"events", "alert.event.query"}, {"history", "alert.history.query"}, {"detail", "alert.event.get"}} {
			data := map[string]any{"resources": base["resources"]}
			if view.name == "detail" {
				data["instance"], data["events"] = object(instance), []any{}
			} else {
				data["page"] = object(alerting.Page[alerting.Instance]{Items: []alerting.Instance{instance}, Complete: true, Total: 1})
			}
			out["event-kind-"+kind+"-"+view.name] = Fixture{Tool: view.tool, Data: data, Expected: []string{"Host resource pressure"}}
		}
	}
	for _, kind := range []string{"table", "timeseries", "zero", "null", "empty"} {
		query := "* | stats count() by service_name"
		if kind == "timeseries" {
			query = "* | stats count() by bin(timestamp, 5m), service_name"
		}
		plan, err := kql.CompileQuery("logs", kql.Request{Expression: query, Start: now.Add(-1800e9), End: now, Scope: kql.Scope{ResourceIDs: []uuid.UUID{host}}, Budget: kql.Budget{MaxRows: 100}})
		if err != nil {
			panic(err)
		}
		row := map[string]any{"group_value": "orders-api", "count": 427}
		if kind == "timeseries" {
			row["timestamp"] = now
		}
		if kind == "zero" {
			row["count"] = 0
		}
		if kind == "null" {
			row["count"] = nil
		}
		rows := []any{row}
		expected := []string{"orders-api"}
		if kind == "empty" {
			rows = []any{}
			expected = []string{}
		}
		data := map[string]any{"schema_version": "argus.kql_result/v1", "result_type": plan.ResultType, "data": rows, "meta": map[string]any{}, "resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}}, "query_context": map[string]any{"query": query, "from": now.Add(-1800e9), "to": now}}
		out["log-aggregate-"+kind] = Fixture{Tool: "telemetry.kql.query", Data: data, Expected: expected}
	}
	trace := Normal("trace-result")["data"].(map[string]any)["queryTrace"].(map[string]any)
	fields := `traceId rootService rootOperation startTime duration spanCount errorCount completeness spans { spanId parentSpanId serviceName operationName startTime duration status }`
	apmFields := `basis percentileMethod status coverage { observedSpanCount requestSampleCount } rows { sourceId resourceId serviceName instanceId instanceName operationName timestamp samplesPerSecond sampleCount errorCount errorRate durationP95Ms }`
	apm := map[string]any{"basis": "received_entry_spans", "percentileMethod": "tdigest", "status": "ready", "coverage": map[string]any{"observedSpanCount": 427, "requestSampleCount": 427}, "rows": []any{map[string]any{"sourceId": "20000000-0000-4000-8000-000000000001", "resourceId": host, "serviceName": "orders-api", "instanceId": "orders-1", "instanceName": "orders-primary", "operationName": "POST /orders", "timestamp": now, "samplesPerSecond": 7.12, "sampleCount": 427, "errorCount": 7, "errorRate": 0.0164, "durationP95Ms": 73.5}}}
	for _, root := range []string{"queryBasicTraces", "queryBasicTracesByName", "queryTraces", "queryTrace", "queryTraceGraph", "queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED", "queryAPMTopology"} {
		args, selection := "", `traces { `+fields+` }`
		var value any = map[string]any{"traces": []any{trace}, "total": 1}
		switch root {
		case "queryTrace":
			args = `(traceId:"trace-001")`
			selection = fields
			value = trace
		case "queryTraceGraph":
			args = `(traceId:"trace-001",sourceId:"20000000-0000-4000-8000-000000000001",resourceId:"` + host.String() + `")`
			selection = `traceId completeness missingParentCount spans { spanId parentSpanId serviceName operationName startTime duration status } edges { parentSpanId childSpanId }`
			value = map[string]any{"traceId": "trace-001", "completeness": "complete", "spans": trace["spans"], "edges": []any{}}
		case "queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED":
			selection = apmFields
			value = apm
			if root == "queryAPMRED" {
				args = "(bucketSeconds:60)"
			}
		case "queryAPMTopology":
			selection = `basis percentileMethod status nodes { id sourceId resourceId serviceName } edges { sourceNodeId targetNodeId sampleCount errorCount durationP95Ms }`
			value = map[string]any{"basis": "received_entry_spans", "percentileMethod": "tdigest", "status": "ready", "nodes": []any{map[string]any{"id": "orders-node", "resourceId": host, "sourceId": "source-db", "serviceName": "orders-api"}, map[string]any{"id": "db-node", "resourceId": host, "sourceId": "source-db", "serviceName": "postgresql"}}, "edges": []any{map[string]any{"sourceNodeId": "orders-node", "targetNodeId": "db-node", "sampleCount": 427, "errorCount": 7, "durationP95Ms": 73.5}}}
		}
		document := "query { " + root + args + " { " + selection + " } }"
		if err := skywalking.ValidateInputs(document, "", nil); err != nil {
			panic(err)
		}
		data := map[string]any{"query_context": map[string]any{"document": document, "from": now.Add(-1800e9), "to": now}, "data": map[string]any{root: value}, "resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}}}
		out["trace-root-"+root] = Fixture{Tool: "telemetry.skywalking.trace", Data: object(data), Expected: []string{"orders-api"}}
	}
	document := `query Combined { services: queryAPMServices { rows { label: serviceName samples: sampleCount } } traces: queryBasicTracesByName(serviceName:"orders-api") { traces { ...TraceFields } } } fragment TraceFields on Trace { id:traceId rootService rootOperation duration }`
	if err := skywalking.ValidateInputs(document, "Combined", nil); err != nil {
		panic(err)
	}
	out["trace-alias-multi"] = Fixture{Tool: "telemetry.skywalking.trace", Data: object(map[string]any{"query_context": map[string]any{"document": document, "operation_name": "Combined"}, "data": map[string]any{"services": map[string]any{"rows": []any{map[string]any{"label": "orders-api", "samples": 427}}}, "traces": map[string]any{"traces": []any{map[string]any{"id": "trace-alias", "rootService": "orders-api", "rootOperation": "POST /orders", "duration": 42}}}}}), Expected: []string{"orders-api", "trace-alias", "427"}}
	return out
}
