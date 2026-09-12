//go:build !windows

package collectormanager

import (
	"context"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func (Manager) applyWindowsLocal(context.Context, *connectorv1.CollectorManagementCommand) (Result, error) {
	return Result{}, ErrUnsupportedPlatform
}
