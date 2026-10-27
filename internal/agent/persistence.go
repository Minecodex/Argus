package agent

import (
	"context"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func persistModelDispatch(ctx context.Context, store *postgres.Store, params db.MarkModelDispatchedParams) error {
	return fencedModelRecordTransaction(ctx, store, func(q *db.Queries) error {
		if err := q.MarkModelDispatched(ctx, params); err != nil {
			return err
		}
		return q.RecordRemoteModelDispatchHash(ctx, db.RecordRemoteModelDispatchHashParams{ModelCallID: params.ID, EnterpriseID: params.EnterpriseID})
	})
}
func persistStepStatus(ctx context.Context, store *postgres.Store, params db.FinishRunStepParams) error {
	return fencedModelRecordTransaction(ctx, store, func(q *db.Queries) error { _, err := q.FinishRunStep(ctx, params); return err })
}

// Model records only update already-owned call/step rows and their provenance.
// Row locks retain atomicity and the task fence without SSI predicate conflicts
// from concurrent readers. Provider requests and business callbacks never run
// in this transaction, and are never retried by it.
func fencedModelRecordTransaction(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
	return store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if err := lockTaskLease(ctx, q); err != nil {
			return err
		}
		return fn(q)
	})
}
func persistModelOutcome(ctx context.Context, store *postgres.Store, params db.FinishModelCallParams) error {
	return fencedModelRecordTransaction(ctx, store, func(q *db.Queries) error { _, err := q.FinishModelCall(ctx, params); return err })
}
