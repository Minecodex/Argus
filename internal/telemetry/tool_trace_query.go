package telemetry

import (
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

var traceApplicationViews = []string{"services", "instances", "endpoints", "red", "topology", "traces"}

func traceToolInputError(path, constraint string) error {
	return toolruntime.Error{Kind: "TOOL_INPUT_INVALID", Message: "Use an application_view for APM, or a document from the supported Argus Trace schema. No query was executed.", Details: map[string]any{
		"schema_validation": true, "issues": []map[string]any{{"path": "/arguments/" + path, "constraint": constraint}},
		"supported_application_views": traceApplicationViews, "supported_trace_roots": []string{"queryTraces", "queryTrace", "queryBasicTraces", "queryBasicTracesByName"},
	}}
}

func traceToolQuery(input map[string]any, limit int) (string, string, map[string]any, error) {
	view, hasView := input["application_view"].(string)
	if !hasView {
		for _, name := range []string{"application_view", "service_ids", "bucket_seconds"} {
			if _, present := input[name]; present {
				return "", "", nil, traceToolInputError(name, "typed_application_view_required")
			}
		}
		document, ok := input["document"].(string)
		if !ok || strings.TrimSpace(document) == "" {
			return "", "", nil, traceToolInputError("document", "required_document_or_application_view")
		}
		operation, variables, err := traceToolParameters(input)
		if err != nil {
			return "", "", nil, traceToolInputError("variables", "bounded_trace_parameters")
		}
		if err = skywalking.ValidateToolDocument(document, operation, variables); err != nil {
			return "", "", nil, traceToolInputError("document", "supported_argus_trace_document")
		}
		return document, operation, variables, nil
	}
	for _, name := range []string{"document", "operation_name", "variables"} {
		if _, present := input[name]; present {
			return "", "", nil, traceToolInputError(name, "exclusive_with_application_view")
		}
	}
	// limit remains the common execution row budget. APM aggregates also
	// account for their coverage envelope; do not request a page that already
	// consumes that row. Never increase the caller's budget to fit a page.
	minimum := 2
	if view == "traces" {
		minimum = 1
	}
	if limit < minimum || limit > 100000 {
		return "", "", nil, traceToolInputError("limit", "application_view_total_row_budget")
	}
	page := min(100, limit-minimum+1)
	document, err := skywalking.ApplicationAPMQuery(view, page)
	if err != nil || view == "trace_detail" {
		return "", "", nil, traceToolInputError("application_view", "supported_application_view")
	}
	variables := map[string]any{"serviceIds": nil}
	if services, present := input["service_ids"]; present {
		ids, err := inputUUIDs(services)
		if err != nil || len(ids) == 0 || len(ids) > 200 {
			return "", "", nil, traceToolInputError("service_ids", "nonempty_service_ids")
		}
		names := make([]any, 0, len(ids))
		for _, id := range ids {
			names = append(names, id.String())
		}
		variables["serviceIds"] = names
	}
	if view == "red" {
		seconds := 60
		if value, present := input["bucket_seconds"]; present {
			number, ok := value.(float64)
			if !ok || number != float64(int(number)) || number < 60 || number > 3600 {
				return "", "", nil, traceToolInputError("bucket_seconds", "integer_60_to_3600")
			}
			seconds = int(number)
		}
		variables["bucketSeconds"] = float64(seconds)
	} else if _, present := input["bucket_seconds"]; present {
		return "", "", nil, traceToolInputError("bucket_seconds", "red_view_only")
	}
	if err = skywalking.ValidateToolDocument(document, "ApplicationAPM", variables); err != nil {
		return "", "", nil, traceToolInputError("application_view", "valid_application_parameters")
	}
	return document, "ApplicationAPM", variables, nil
}

func traceToolQuerySchema(base map[string]any) map[string]any {
	properties := base["properties"].(map[string]any)
	properties["document"].(map[string]any)["description"] = "Argus supports queryTraces/queryTrace/queryAPM* and flat-argument queryBasicTraces. Generic SkyWalking condition/queryDuration/segmentId vocabulary is not supported. Prefer application_view for APM. / 应用性能优先使用 application_view；不要套用上游不适用的参数。"
	properties["application_view"] = map[string]any{"type": "string", "enum": traceApplicationViews, "description": "Typed application APM query using the same versioned vocabulary as the portal. Resource IDs remain explicit and authorization is unchanged. / 应用性能视图，与门户共用查询及授权。"}
	properties["service_ids"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 200, "uniqueItems": true, "items": map[string]any{"type": "string", "format": "uuid"}}
	properties["limit"].(map[string]any)["description"] = "Total execution row budget, including APM coverage and topology relations; default 5000. APM list pages use at most 100 rows within this budget. Aggregate views require at least 2. / 查询总行数预算含统计信息与拓扑关系，默认 5000；APM 列表在预算内最多取 100 行，聚合视图至少需要 2 行预算。"
	properties["bucket_seconds"] = map[string]any{"type": "integer", "minimum": 60, "maximum": 3600, "description": "RED view only; default 60 seconds. / 仅 RED 视图使用，默认 60 秒。"}
	base["required"] = []string{"resource_ids", "from", "to"}
	base["oneOf"] = []map[string]any{
		{"required": []string{"document"}, "not": map[string]any{"anyOf": []map[string]any{{"required": []string{"application_view"}}, {"required": []string{"service_ids"}}, {"required": []string{"bucket_seconds"}}}}},
		{"required": []string{"application_view"}, "not": map[string]any{"anyOf": []map[string]any{{"required": []string{"document"}}, {"required": []string{"operation_name"}}, {"required": []string{"variables"}}}}},
	}
	return base
}
