package kubernetesworkbench

import (
	"context"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// The workbench is the outer extension in both Server and Worker. Forward
// optional preparation even when this extension does not own the action.
func (e ActionExtension) PrepareActionExecution(ctx context.Context, action db.PendingAction, raw json.RawMessage) (context.Context, error) {
	if next, ok := e.Next.(resource.ActionExecutionPreparer); ok {
		return next.PrepareActionExecution(ctx, action, raw)
	}
	return ctx, nil
}
