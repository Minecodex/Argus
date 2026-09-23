package conversation

import (
	"context"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// CancelRunRecord runs in the caller's transaction while its Run row is locked.
// The status and terminal event become visible together; execution watchers
// cancel in-flight requests while retaining their authoritative final receipts.
func CancelRunRecord(ctx context.Context, q *db.Queries, run db.Run, reason, actorType, actorID string) (db.Run, error) {
	return FinishRunRecord(ctx, q, run, "cancelled", reason, "", actorType, actorID)
}
