package agent

import (
	"context"
	"time"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Requests outlive browser streams. Watch durable cancellation and identity
// state to stop model generation, revoke sandbox commands, and notify MCP.
func (loop Loop) toolContext(parent context.Context, run db.Run) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check, done := context.WithTimeout(ctx, 2*time.Second)
				current, err := loop.Store.Queries.GetRun(check, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
				if err == nil && !terminalRun(current.Status) {
					user, identityErr := loop.Store.Queries.GetEnterpriseUser(check, db.GetEnterpriseUserParams{ID: run.ActorUserID, EnterpriseID: run.EnterpriseID})
					if identityErr != nil || user.Status != "active" || user.AuthorizationVersion != run.AuthorizationVersion {
						err = context.Canceled
					}
				}
				done()
				if err != nil || terminalRun(current.Status) {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
