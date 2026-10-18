package connector

import (
	"context"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"strings"
)

func (c connectorClient) executeCommand(ctx context.Context, command *connectorv1.ConnectorCommand, credential, operationSecret []byte, completed chan<- completedCommand) {
	if !strings.HasPrefix(command.CommandType, "remote_execution_") {
		executeConnectorCommand(ctx, command, credential, operationSecret, completed)
		return
	}
	defer clear(credential)
	defer clear(operationSecret)
	// mTLS reconnects do not cancel an approved batch or replace its interpreter.
	root := c.executionContext
	if root == nil {
		root = ctx
	}
	outcome := (commandExecutor{Remote: c.remote}).execute(root, command, credential, operationSecret)
	if err := c.store.saveResult(outcomeRecord(command, outcome)); err != nil {
		outcome = commandOutcome{code: "REMOTE_RESULT_UNKNOWN"}
	}
	select {
	case completed <- completedCommand{command: command, outcome: outcome}:
	case <-ctx.Done():
	}
}
