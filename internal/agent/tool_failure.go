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
	permitted := code == "TOOL_DESCRIPTION_REQUIRED" || code == "TOOL_INPUT_INVALID" && safe.Details["schema_validation"] == true || code == "REMOTE_CONTEXT_LOST" && safe.Details["requires_recovery"] == true
	if permitted {
		data["message"], data["details"] = safe.Message, safe.Details
	}
	return data
}
