package component

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestDashboardWorkspaceSourceDenialsKeepFileAccessContract(t *testing.T) {
	for _, cause := range []error{dashboard.ErrDenied, dashboard.ErrArchived, dashboard.ErrNotFound} {
		err := dashboardWorkspaceSourceError(fmt.Errorf("source: %w", cause))
		var typed toolruntime.Error
		if !errors.As(err, &typed) || typed.Kind != "WORKSPACE_FILE_FORBIDDEN" {
			t.Fatalf("source denial became internal error: %v", err)
		}
	}
	if err := dashboardWorkspaceSourceError(nil); err != nil {
		t.Fatal(err)
	}
	transient := errors.New("temporary source store failure")
	if err := dashboardWorkspaceSourceError(transient); err != transient {
		t.Fatal("transient failure mislabeled as an authorization denial")
	}
}
