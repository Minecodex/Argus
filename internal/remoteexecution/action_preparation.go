package remoteexecution

import (
	"context"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) PrepareActionExecution(ctx context.Context, action db.PendingAction, raw json.RawMessage) (context.Context, error) {
	if next, ok := s.Next.(resource.ActionExecutionPreparer); ok {
		return next.PrepareActionExecution(ctx, action, raw)
	}
	return ctx, nil
}
