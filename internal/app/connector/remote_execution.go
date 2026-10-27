package connector

import (
	"context"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"strings"
)

func (c connectorClient) executeCommand(ctx context.Context, command *connectorv1.ConnectorCommand, credential, operationSecret []byte, completed chan<- completedCommand) {
	if command.CommandType != "collector_management" && !strings.HasPrefix(command.CommandType, "remote_execution_") {
		executeConnectorCommand(ctx, command, credential, operationSecret, completed)
		return
	}
	defer clear(credential)
	defer clear(operationSecret)
	// Approved batches and Collector management outlive a transport reconnect.
	// The process lifetime and executor timeout still bound every operation.
	root := c.executionContext
	if root == nil {
		root = ctx
	}
	outcome := (commandExecutor{Remote: c.remote}).execute(root, command, credential, operationSecret)
	if err := c.store.saveResult(outcomeRecord(command, outcome)); err != nil {
		code := "REMOTE_RESULT_UNKNOWN"
		if command.CommandType == "collector_management" {
			code = "EXECUTION_RESULT_UNKNOWN"
		}
		outcome = commandOutcome{code: code}
	}
	select {
	case completed <- completedCommand{command: command, outcome: outcome}:
	case <-ctx.Done():
	}
}
