package dashboard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const apmProjection = `basis percentileMethod groupBy status windowStart windowEnd limited
 coverage {observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount}
 rows {sourceId resourceId serviceName instanceId instanceName operationName timestamp intervalSeconds observedSpanCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP50Ms durationP95Ms durationP99Ms}`

const topologyProjection = `basis percentileMethod status windowStart windowEnd limited
 coverage{observedSpanCount missingParentCount ambiguousParentCount cyclicSpanCount missingServiceCount unknownSourceCount}
 nodes{id sourceId resourceId serviceName}
 edges{sourceNodeId targetNodeId observedEdgeCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP95Ms}`

func compileAPMBuilder(builder Builder) (DSL, error) {
	if builder.Metric != "" || builder.MetricType != "" || len(builder.GroupBy) > 0 || builder.WindowSeconds != 0 || builder.TopN != 0 || builder.TraceID != "" || builder.Operation != "apm_red" && builder.BucketSeconds != 0 {
		return DSL{}, fmt.Errorf("APM builder contains incompatible fields")
	}
	root := map[string]string{"apm_services": "queryAPMServices", "apm_instances": "queryAPMInstances", "apm_endpoints": "queryAPMEndpoints", "apm_red": "queryAPMRED", "apm_topology": "queryAPMTopology"}[builder.Operation]
	if root == "" {
		return DSL{}, fmt.Errorf("unsupported APM operation")
	}
	args := []string{}
	attributes := []string{}
	seen := map[string]bool{}
	for _, f := range builder.Filters {
		if apmAttributeField(f.Field) {
			if f.Variable != "" || f.LocalParameter != "" || f.Operator != "=" && f.Operator != "!=" {
				return DSL{}, fmt.Errorf("APM attribute filter requires bound equality/set values")
			}
			key := strings.TrimPrefix(f.Field, "attributes.")
			if strings.HasPrefix(f.Field, "resource_attributes.") {
				key = strings.TrimPrefix(f.Field, "resource_attributes.")
			}
			values := f.Values
			if len(values) == 0 {
				values = []string{f.Value}
			}
			quoted := []string{}
			for _, v := range values {
				quoted = append(quoted, quoteGraphQL(v))
			}
			attributes = append(attributes, "{resource:"+strconv.FormatBool(strings.HasPrefix(f.Field, "resource_attributes."))+",key:"+quoteGraphQL(key)+",values:["+strings.Join(quoted, ",")+"],negate:"+strconv.FormatBool(f.Operator == "!=")+"}")
			continue
		}
		if f.Operator != "=" || len(f.Values) > 1 || f.Variable != "" || f.LocalParameter != "" || seen[f.Field] {
			return DSL{}, fmt.Errorf("APM filters require distinct scalar equality inputs")
		}
		seen[f.Field] = true
		switch f.Field {
		case "serviceName", "serviceInstanceName", "operationName", "sourceId", "resourceId":
		default:
			return DSL{}, fmt.Errorf("unsupported APM filter")
		}
		value := f.Value
		if len(f.Values) == 1 {
			value = f.Values[0]
		}
		args = append(args, f.Field+":"+quoteGraphQL(value))
	}
	if len(attributes) > 0 {
		args = append(args, "filters:["+strings.Join(attributes, ",")+"]")
	}
	if builder.Operation == "apm_red" {
		if builder.BucketSeconds < 1 || builder.BucketSeconds > 86400 {
			return DSL{}, fmt.Errorf("APM bucket is required")
		}
		args = append(args, "bucketSeconds:"+strconv.Itoa(builder.BucketSeconds))
	}
	if builder.Limit < 0 || builder.Limit > 50000 {
		return DSL{}, fmt.Errorf("invalid APM limit")
	}
	if builder.Limit > 0 {
		args = append(args, "limit:"+strconv.Itoa(builder.Limit))
	}
	parameters := ""
	if len(args) > 0 {
		parameters = "(" + strings.Join(args, ",") + ")"
	}
	projection := apmProjection
	if builder.Operation == "apm_topology" {
		projection = topologyProjection
	}
	return DSL{Expression: "query {" + root + parameters + " {" + projection + "}}"}, nil
}

func quoteGraphQL(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

func apmAttributeField(field string) bool {
	return strings.HasPrefix(field, "attributes.") || strings.HasPrefix(field, "resource_attributes.")
}
