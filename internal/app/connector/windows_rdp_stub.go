//go:build !windows

package connector

import (
	"context"
	"errors"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func configureWindowsRDP(context.Context, *connectorv1.HostWindowsRDPConfigure) (*connectorv1.HostWindowsRDPConfigureResult, error) {
	return nil, errors.New("Windows RDP configuration is unavailable on this platform")
}
