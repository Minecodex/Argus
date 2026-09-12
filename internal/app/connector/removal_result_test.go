package connector

import (
	"github.com/kakj-go/Argus/internal/connectorprotocol"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"testing"
)

func TestRemovalCanSendRunningAndFailureWithoutClosingStream(t *testing.T) {
	running, err := emptyCommandResult("host_connector_removal")
	if err != nil || !connectorprotocol.ResultAllowed("host_connector_removal", running) {
		t.Fatalf("removal running frame rejected before SSH execution: %v", err)
	}
	command := &connectorv1.ConnectorCommand{CommandType: "host_connector_removal", CommandId: "cmd_removal"}
	frame := commandResultFrame(command, 3, commandOutcome{code: "HOST_REMOVAL_TARGET_UNREACHABLE"}).GetCommandResult()
	if frame == nil || frame.Status != "failed" || frame.ConnectionEpoch != 3 || !connectorprotocol.ResultAllowed(command.CommandType, frame.TypedResult) {
		t.Fatal("failed removal cannot be reported on the existing stream")
	}
	if connectorprotocol.ResultAllowed("unknown", running) || connectorprotocol.ResultAllowed("host_connector_install", running) {
		t.Fatal("removal result accepted for a different command")
	}
}
