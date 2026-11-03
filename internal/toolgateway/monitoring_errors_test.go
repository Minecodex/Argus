package toolgateway

import (
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestMonitoringToolErrorKeepsOnlyPublicCode(t *testing.T) {
	var coded toolruntime.Error
	for _, known := range []error{monitoring.ErrInvalid, monitoring.ErrConflict, monitoring.ErrCredential} {
		err := monitoringToolError(fmt.Errorf("private backend detail: %w", known))
		if !errors.As(err, &coded) || coded.Code() != known.Error() || err.Error() != known.Error() {
			t.Fatalf("public error: %v", err)
		}
	}
}
