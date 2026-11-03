package agent

import (
	"errors"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func modelToolFailure(cause error, code string) map[string]any {
	data := map[string]any{"error_code": code, "message": "Tool invocation failed."}
	var safe toolruntime.Error
	if !errors.As(cause, &safe) {
		return data
	}
	permitted := code == "TOOL_SEARCH_REQUIRED" && safe.Details["requires_search"] == true || code == "TOOL_DESCRIPTION_REQUIRED" || code == "TOOL_INPUT_INVALID" && safe.Details["schema_validation"] == true || code == "TOOL_QUERY_INVALID" && safe.Details["query_validation"] == true || code == "REMOTE_CONTEXT_LOST" && safe.Details["requires_recovery"] == true || code == "TOOL_QUERY_BUDGET_EXCEEDED" && safe.Details["query_budget"] == true
	permitted = permitted || code == "DASHBOARD_CONDITION_EVIDENCE_REQUIRED" && safe.Details["condition_evidence"] == true || code == "DASHBOARD_RESOURCE_SCOPE_REQUIRED" && safe.Details["resource_condition_required"] == true
	if permitted {
		data["message"], data["details"] = safe.Message, safe.Details
	}
	return data
}
