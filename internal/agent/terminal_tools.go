package agent

import (
	"context"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (loop Loop) reconcileTerminalToolCalls(ctx context.Context, run db.Run) error {
	calls, err := loop.Store.Queries.ListUnfinishedRunToolCalls(ctx, db.ListUnfinishedRunToolCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return err
	}
	for _, call := range calls {
		result := toolruntime.Result{}
		if call.Status == "dispatched" && (call.Source == "external_mcp" || call.Source == "sandbox" && call.ToolID != "read") {
			result.Unknown = true
		}
		if _, err := loop.persistToolOutcome(ctx, run, call, result, toolruntime.Error{Kind: "TOOL_CANCELLED"}); err != nil {
			return err
		}
	}
	return nil
}
