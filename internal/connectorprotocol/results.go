// Package connectorprotocol shares wire contracts between Gateway and Connector.
package connectorprotocol

import (
	"errors"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Result is also used for the initial running frame and failed outcomes. A
// command must not disconnect before execution merely because its result
// factory was registered on only one side of the stream.
func Result(command string) (proto.Message, error) {
	switch command {
	case "host_connection_probe":
		return &connectorv1.HostConnectionProbeResult{}, nil
	case "kubernetes_connection_probe":
		return &connectorv1.KubernetesConnectionProbeResult{}, nil
	case "kubernetes_resource_query":
		return &connectorv1.KubernetesResourceQueryResult{}, nil
	case "kubernetes_pod_logs":
		return &connectorv1.KubernetesPodLogsResult{}, nil
	case "connector_uninstall":
		return &connectorv1.ConnectorUninstallResult{}, nil
	case "collector_management":
		return &connectorv1.CollectorManagementResult{}, nil
	case "host_connector_install":
		return &connectorv1.HostConnectorInstallResult{}, nil
	case "host_connector_removal":
		return &connectorv1.HostConnectorRemovalResult{}, nil
	case "host_windows_rdp_configure":
		return &connectorv1.HostWindowsRDPConfigureResult{}, nil
	default:
		return nil, errors.New("unsupported Connector command result type")
	}
}

func ResultAllowed(command string, value *anypb.Any) bool {
	expected, err := Result(command)
	return err == nil && value != nil && value.GetTypeUrl() == "type.googleapis.com/"+string(expected.ProtoReflect().Descriptor().FullName())
}
