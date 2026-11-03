package dashboard

import (
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Only condition paths supplied by this authorized request enter the public
// feedback. Never append database/compiler diagnostics to these messages.
func conditionEvidenceRequired(paths []string) toolruntime.Error {
	required := slices.Clone(paths)
	slices.Sort(required)
	return toolruntime.Error{
		Kind:    "DASHBOARD_CONDITION_EVIDENCE_REQUIRED",
		Message: "Provide one evidence entry per changed condition. Paths are condition-relative: /time, /resources, /variables/<name>, /local_values/<panel>/<name>, or / for reset_all. Cite an exact excerpt from the current user message, or use origin=ai_selection with a reason for a permitted AI choice. Preserve all requested resource and time conditions while correcting evidence; removing a condition does not resolve the user's request.",
		Details: map[string]any{"condition_evidence": true, "required_paths": required, "preserve_user_conditions": true},
	}
}

// A fresh analysis must attribute its resource choice. A saved resource
// selection or an explicit resource/all reset may be inherited unchanged.
func requireAnalysisResourceCondition(state dashboardparams.State) error {
	confirmed := func(path string) bool {
		ref, ok := state.Evidence[path]
		return ok && ref.EventID != uuid.Nil && (ref.Origin == "message_interpretation" || ref.Origin == "ai_selection")
	}
	if confirmed("/resources") || state.Overrides.Resources == nil && confirmed("/") {
		return nil
	}
	return toolruntime.Error{
		Kind:    "DASHBOARD_RESOURCE_SCOPE_REQUIRED",
		Message: "Choose the resources for this analysis explicitly, or inherit an already confirmed resource condition. For requested objects set changes.resources={all:false,ids:[authorized IDs]}; choose all:true with ids:[] only when the task calls for all authorized resources. Add evidence at /resources using the current user text or a permitted AI choice with a reason. Keep the requested time and other conditions.",
		Details: map[string]any{"resource_condition_required": true, "condition_path": "/resources", "preserve_user_conditions": true},
	}
}
