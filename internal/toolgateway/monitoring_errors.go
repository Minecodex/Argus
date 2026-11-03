package toolgateway

import (
	"errors"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Expose only fixed public domain codes. Never forward an underlying database,
// target response or credential error string to the model.
func monitoringToolError(err error) error {
	if errors.Is(err, monitoring.ErrDenied) {
		return toolruntime.Error{Kind: "TOOL_PERMISSION_DENIED"}
	}
	for _, known := range []error{monitoring.ErrInvalid, monitoring.ErrNotFound, monitoring.ErrConflict, monitoring.ErrUnavailable, monitoring.ErrUnsupported, monitoring.ErrDependency, monitoring.ErrServiceLimit, monitoring.ErrCredential, monitoring.ErrLogOverlap} {
		if errors.Is(err, known) {
			return toolruntime.Error{Kind: known.Error()}
		}
	}
	return err
}
