package agent

import (
	"context"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func persistModelDispatch(ctx context.Context, store *postgres.Store, params db.MarkModelDispatchedParams) error {
	return fencedTransaction(ctx, store, func(q *db.Queries) error { return q.MarkModelDispatched(ctx, params) })
}
func persistStepStatus(ctx context.Context, store *postgres.Store, params db.FinishRunStepParams) error {
	return fencedTransaction(ctx, store, func(q *db.Queries) error { _, err := q.FinishRunStep(ctx, params); return err })
}
