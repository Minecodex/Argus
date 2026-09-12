package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/resource"
)

func TestHostRemovalErrorsDistinguishTargetStateAndInfrastructure(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"unknown or unauthorized target", hostremoval.ErrInvalidTarget, "RESOURCE_NOT_FOUND", http.StatusNotFound},
		{"not enrolled", hostremoval.ErrNotInstalled, "HOST_REMOVAL_NOT_INSTALLED", http.StatusConflict},
		{"stale resource", resource.ErrVersionConflict, "VERSION_CONFLICT", http.StatusConflict},
		{"existing operation", hostremoval.ErrOperationState, "HOST_REMOVAL_STATE_CONFLICT", http.StatusConflict},
		{"database unavailable", errors.New("database unavailable"), "INTERNAL_ERROR", http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := hostErrorBase(context.Background(), test.err)
			if body.Code != test.code || hostRemovalStatus(test.err) != test.status {
				t.Fatalf("got %s/%d, want %s/%d", body.Code, hostRemovalStatus(test.err), test.code, test.status)
			}
		})
	}
}
