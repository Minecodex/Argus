package tooltemplatefixture

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"time"
)

const apmCompleteFields = `basis percentileMethod status windowStart windowEnd coverage { observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount } rows { sourceId resourceId serviceName instanceId instanceName operationName timestamp intervalSeconds samplesPerSecond sampleCount errorCount errorRate durationP95Ms }`

func completenessREDRows(groups int) []any {
	rows := []any{}
	for group := 0; group < groups; group++ {
		resource := host.String()
		if group > 0 {
			resource = uuid.NewSHA1(host, []byte("red-host-two")).String()
		}
		for i := 0; i < 600; i++ {
			row := map[string]any{"sourceId": "20000000-0000-4000-8000-000000000001", "resourceId": resource, "serviceName": "orders-api", "instanceId": fmt.Sprint("instance-", group), "instanceName": fmt.Sprint("orders-", group), "operationName": "POST /orders", "timestamp": now.Add(-600 * time.Second).Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano), "intervalSeconds": 1, "samplesPerSecond": float64(i%19 + 1), "sampleCount": i%19 + 1, "errorCount": 1, "errorRate": 0.01, "durationP95Ms": float64(i%101 + 1)}
			if i == 317 {
				row["samplesPerSecond"] = 999.0
			}
			if i == 419 {
				row["errorRate"] = 0.99
			}
			if i == 511 {
				row["durationP95Ms"] = 9999.0
			}
			if i == 200 {
				row["samplesPerSecond"] = nil
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func apmCompletenessScenarios() map[string]Fixture {
	out := map[string]Fixture{}
	resources := []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}, map[string]any{"id": uuid.NewSHA1(host, []byte("red-host-two")), "name": "db-prod-02", "type": "host"}}
	rootValue := func(rows []any) map[string]any {
		return map[string]any{"basis": "received_entry_spans", "percentileMethod": "tdigest", "status": "available", "windowStart": now.Add(-600 * time.Second), "windowEnd": now, "coverage": map[string]any{"observedSpanCount": 1200, "requestSampleCount": 1200}, "rows": rows}
	}
	bind := func(document, root string, value map[string]any) map[string]any {
		if err := skywalking.ValidateInputs(document, "", nil); err != nil {
			panic(err)
		}
		return object(map[string]any{"query_context": map[string]any{"document": document, "from": now.Add(-600 * time.Second), "to": now}, "data": map[string]any{root: value}, "resources": resources, "extensions": map[string]any{"argus": map[string]any{"partial": false}}, "partial": false})
	}
	for _, root := range []string{"queryAPMServices", "queryAPMInstances", "queryAPMEndpoints", "queryAPMRED", "queryAPMTopology"} {
		for _, state := range []string{"true", "false", "unknown"} {
			fields, args := apmCompleteFields, "(limit:1)"
			if root == "queryAPMRED" {
				args = "(bucketSeconds:1,limit:1)"
			}
			value := rootValue(completenessREDRows(1)[:1])
			if root == "queryAPMTopology" {
				fields = `basis percentileMethod status windowStart windowEnd nodes { id sourceId resourceId serviceName } edges { sourceNodeId targetNodeId sampleCount errorCount durationP95Ms }`
				delete(value, "rows")
				value["nodes"], value["edges"] = []any{map[string]any{"id": "orders-node", "sourceId": "source", "resourceId": host, "serviceName": "orders-api"}}, []any{}
			}
			if state != "unknown" {
				fields += " limited"
				value["limited"] = state == "true"
			}
			document := "query { " + root + args + " { " + fields + " } }"
			out["apm-limit-"+root+"-"+state] = Fixture{Tool: "telemetry.skywalking.trace", Data: bind(document, root, value), Expected: []string{"orders-api"}}
		}
	}
	for _, group := range []struct {
		name  string
		count int
	}{{"single", 1}, {"multi", 2}} {
		value := rootValue(completenessREDRows(group.count))
		value["limited"] = false
		document := "query { queryAPMRED(bucketSeconds:1,limit:2000) { " + apmCompleteFields + " limited } }"
		out["apm-red-large-"+group.name] = Fixture{Tool: "telemetry.skywalking.trace", Data: bind(document, "queryAPMRED", value), Expected: []string{"orders-api", "db-prod-01"}}
	}
	partial := clone(out["apm-limit-queryAPMServices-false"].Data)
	partial["partial"] = true
	out["apm-execution-partial"] = Fixture{Tool: "telemetry.skywalking.trace", Data: partial, Expected: []string{"orders-api"}}
	dashboard := Normal("dashboard-result")
	value := rootValue(completenessREDRows(2))
	value["limited"] = true
	dashboard["panels"] = []any{map[string]any{"id": "apm-red", "name": "APM RED frozen overview", "type": "apm_red", "signal": "traces", "status": "success", "legend": true, "layout": map[string]any{"x": 0, "y": 0, "w": 12, "h": 1}, "targets": []any{map[string]any{"id": "red", "status": "success", "result_type": "apm_red", "data": map[string]any{"data": map[string]any{"queryAPMRED": value}}}}}}
	presentation.DashboardObservationContext(dashboard, nil)
	out["dashboard-apm-large"] = Fixture{Tool: "telemetry.dashboard.query", Data: dashboard, Expected: []string{"APM RED frozen overview", "db-prod-01"}}
	return out
}
