package telemetry

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateRouteTransportKeepsExecutionAndTunnelOwnershipOrthogonal(t *testing.T) {
	t.Parallel()
	credentialID := uuid.New()
	directTunnel := collectorTunnelTarget{Initiator: "direct_executor", Address: "192.0.2.10", Port: 22,
		Username: "root", PinnedHostKey: "SHA256:test", CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}, CredentialVersion: 1}
	connectorTunnel := directTunnel
	connectorTunnel.Initiator = "connector"
	connectorTunnel.ConnectorID = uuid.NullUUID{UUID: uuid.New(), Valid: true}

	for _, test := range []struct {
		name      string
		transport string
		routeKind string
		execution string
		target    collectorTunnelTarget
		wantError bool
	}{
		{name: "direct host", transport: "direct", routeKind: "direct_argus", execution: "direct"},
		{name: "direct executor tunnel", transport: "executor_tunnel", routeKind: "direct_argus", execution: "direct", target: directTunnel},
		{name: "mode C local install with executor tunnel", transport: "executor_tunnel", routeKind: "direct_argus", execution: "connector", target: directTunnel},
		{name: "bastion member tunnel", transport: "bastion_tunnel", routeKind: "bastion_gateway", execution: "connector", target: connectorTunnel},
		{name: "control owner cannot carry OTLP", transport: "executor_tunnel", routeKind: "direct_argus", execution: "connector", target: connectorTunnel, wantError: true},
		{name: "member tunnel requires connector", transport: "bastion_tunnel", routeKind: "bastion_gateway", execution: "direct", target: connectorTunnel, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateRouteTransport(test.transport, test.routeKind, "host", test.execution, test.target)
			if (err != nil) != test.wantError {
				t.Fatalf("validateRouteTransport() error = %v, wantError=%t", err, test.wantError)
			}
		})
	}
}

func TestTunnelIdentityLoopbackPortIsAdjacentAndBounded(t *testing.T) {
	port, err := tunnelIdentityLoopbackPort(14317)
	if err != nil || port != 14318 {
		t.Fatalf("identity loopback = %d, err=%v", port, err)
	}
	for _, invalid := range []int{0, 65535} {
		if _, err = tunnelIdentityLoopbackPort(invalid); err == nil {
			t.Fatalf("invalid OTLP loopback port %d was accepted", invalid)
		}
	}
}

func TestTunnelLoopbackRejectsServicePortsForDataAndIdentity(t *testing.T) {
	for _, port := range []int32{21, 22, 4316, 4317, 8442, 8443, 8444, 8445, 8463, 8464, 9442, 9443, 9444, 9445, 9463, 9464, 13132, 13133} {
		if !tunnelLoopbackPortsReserved(port) {
			t.Fatalf("reserved tunnel loopback port %d was accepted", port)
		}
	}
	for _, port := range []int32{14317, 14318, 24317} {
		if tunnelLoopbackPortsReserved(port) {
			t.Fatalf("available tunnel loopback port %d was rejected", port)
		}
	}
}
