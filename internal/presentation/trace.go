package presentation

import "github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"

func normalizeTrace(data, payload map[string]any) {
	if data["result"] != nil {
		views, _ := data["result"].([]any)
		skywalking.RefreshPresentationPreviews(views)
		return
	}
	context, _ := data["query_context"].(map[string]any)
	document, _ := context["document"].(string)
	operation, _ := context["operation_name"].(string)
	if document == "" {
		return
	}
	views, err := skywalking.PresentationResults(document, operation, payload)
	if err != nil {
		data["presentation_error"] = "TRACE_PRESENTATION_INVALID"
		return
	}
	data["result"] = views
	// All exact canonical rows remain in the frozen result collections. Avoid
	// storing a second copy of the same large GraphQL response in this UI binding.
	delete(data, "data")
	delete(data, "items")
}
