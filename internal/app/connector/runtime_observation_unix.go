//go:build !windows

package connector

import (
	"net"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func observeHostRuntime() *connectorv1.HostRuntimeObservation {
	status := "unavailable"
	connection, err := net.DialTimeout("tcp", "127.0.0.1:22", 300*time.Millisecond)
	if err == nil {
		status = "available"
		_ = connection.Close()
	}
	return &connectorv1.HostRuntimeObservation{Platform: "linux", OpensshStatus: status, RdpStatus: "unavailable", ObservedAt: timestamppb.Now()}
}
