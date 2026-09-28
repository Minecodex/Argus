package dashboard

import (
	"fmt"
	"strconv"
	"strings"
)

const traceListProjection = `total traces {traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount rootPresent missingParentCount}`
const traceGraphProjection = `traceId completeness missingParentCount ambiguousParentCount excludedSpanCount
 spans {traceId spanId parentSpanId sourceId resourceId serviceName operationName startTime duration status resourceAttributes attributes events links}
 edges {parentSpanId childSpanId parentSourceId childSourceId parentResourceId childResourceId missingParent cycle}`

func compileTraceBuilder(b Builder) (DSL, error) {
	if b.Operation != "list" && b.Operation != "detail" && b.Operation != "trace_graph" {
		return DSL{}, fmt.Errorf("unsupported trace builder operation")
	}
	args, attributes := []string{}, []string{}
	seen := map[string]bool{}
	if b.TraceID != "" {
		args = append(args, "traceId:"+quoteGraphQL(b.TraceID))
		seen["traceId"] = true
	}
	for _, f := range b.Filters {
		if f.Variable != "" || f.LocalParameter != "" || f.DrilldownInput != "" {
			return DSL{}, fmt.Errorf("trace filter needs bound input")
		}
		if apmAttributeField(f.Field) {
			value, err := compileAttributeInput(f)
			if err != nil {
				return DSL{}, err
			}
			attributes = append(attributes, value)
			continue
		}
		if f.Operator != "=" || len(f.Values) > 1 || seen[f.Field] {
			return DSL{}, fmt.Errorf("trace filter needs distinct scalar inputs")
		}
		seen[f.Field] = true
		value := f.Value
		if len(f.Values) == 1 {
			value = f.Values[0]
		}
		switch f.Field {
		case "serviceName", "serviceInstanceName", "operationName", "status", "sourceId", "resourceId", "traceId":
			args = append(args, f.Field+":"+quoteGraphQL(value))
		case "durationMin", "durationMax":
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				return DSL{}, err
			}
			args = append(args, f.Field+":"+value)
		default:
			return DSL{}, fmt.Errorf("unsupported trace filter")
		}
	}
	if len(attributes) > 0 {
		args = append(args, "filters:["+strings.Join(attributes, ",")+"]")
	}
	if b.Limit < 0 || b.Limit > 50000 {
		return DSL{}, fmt.Errorf("invalid trace list limit")
	}
	if b.Operation == "list" && b.Limit > 0 {
		args = append(args, "pageSize:"+strconv.Itoa(b.Limit))
	}
	root, projection := "queryTraces", traceListProjection
	if b.Operation == "detail" {
		root, projection = "queryTrace", `traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount spans{traceId spanId parentSpanId sourceId resourceId serviceName operationName startTime duration status attributes events links} edges{parentSpanId childSpanId parentService childService depth}`
	}
	if b.Operation == "trace_graph" {
		root, projection = "queryTraceGraph", traceGraphProjection
	}
	parameters := ""
	if len(args) > 0 {
		parameters = "(" + strings.Join(args, ",") + ")"
	}
	return DSL{Expression: "query {" + root + parameters + " {" + projection + "}}"}, nil
}

func compileAttributeInput(f Filter) (string, error) {
	if f.Operator != "=" && f.Operator != "!=" {
		return "", fmt.Errorf("attribute filter requires equality or exclusion")
	}
	key := strings.TrimPrefix(f.Field, "attributes.")
	resource := strings.HasPrefix(f.Field, "resource_attributes.")
	if resource {
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
	return "{resource:" + strconv.FormatBool(resource) + ",key:" + quoteGraphQL(key) + ",values:[" + strings.Join(quoted, ",") + "],negate:" + strconv.FormatBool(f.Operator == "!=") + "}", nil
}
