package workspace

import (
	"context"

	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// DeliverNativeQueryResults imports only committed native query artifacts from
// this active Run. Recovery uses the same immutable ref and canonical path;
// changed bytes are rejected by ImportResult instead of overwritten.
func (service Service) DeliverNativeQueryResults(ctx context.Context, run db.Run) error {
	rows, err := service.Store.Queries.ListUndeliveredNativeQueryResults(ctx, db.ListUndeliveredNativeQueryResultsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID})
	if err != nil || len(rows) == 0 {
		return err
	}
	p, err := (conversation.Service{Store: service.Store}).ToolPrincipal(ctx, run.EnterpriseID, run.ActorUserID, run.ConversationID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		filePath := toolruntime.NativeQueryFile(row.TargetToolID, row.ToolCallID)
		if filePath == "" {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		if _, err := service.ImportResult(ctx, p, run.ID, row.ResultRef, filePath); err != nil {
			return err
		}
	}
	return nil
}
