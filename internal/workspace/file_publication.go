package workspace

import (
	"context"
	"time"

	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Every publication uses the same conversation -> Workspace lock order and
// rechecks the live writer fence before committing any file or result mapping.
func (s Service) publishFile(ctx context.Context, p toolruntime.Principal, access *Access, publish func(*db.Queries, db.Workspace) error) error {
	transaction := func(q *db.Queries) error {
		c, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err != nil {
			return err
		}
		if c.Status != "active" {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
		w, err := q.LockWorkspace(ctx, db.LockWorkspaceParams{ID: access.Workspace.ID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return err
		}
		if w.Status != "ready" || w.FenceToken != access.Workspace.FenceToken || w.LeaseOwner.String != access.owner || !w.LeaseUntil.Valid || !w.LeaseUntil.Time.After(time.Now()) {
			return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
		return publish(q, w)
	}
	// The callback is database-only: a serialization retry must never repeat a
	// file write, consume an input stream, or dispatch a remote command.
	for attempt := 0; ; attempt++ {
		err := s.Store.InTx(ctx, transaction)
		if attempt >= 7 || !postgres.IsRetryableTransactionError(err) {
			return err
		}
		timer := time.NewTimer(time.Duration(20*(attempt+1)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
