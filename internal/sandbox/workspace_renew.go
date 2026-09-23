package sandbox

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"math"
	"time"
)

// RenewWorkspaceSession reserves any additional lifetime under the same quota
// lock as session creation. Idle reuse cannot silently overbook compute quota.
func (service Service) RenewWorkspaceSession(ctx context.Context, id uuid.UUID, runtime WorkspaceRuntime, seconds int) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		session, err := q.GetSandboxSession(ctx, id)
		if err != nil {
			return err
		}
		quota, err := q.GetSandboxQuotaForUpdate(ctx, session.EnterpriseID)
		if err != nil {
			return ErrQuotaExceeded
		}
		session, err = q.GetSandboxSessionForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if session.Status != "running" || session.ProfileID != runtime.Profile.ID || session.ProfileRevision != runtime.Profile.Revision {
			return ErrWorkspaceRuntimeChanged
		}
		if _, err := service.ResolveWorkspaceRuntime(ctx, runtime.Identity()); err != nil {
			return err
		}
		now := time.Now().UTC()
		// Usage is charged to the session's creation month. Rotate a warm
		// instance at the month boundary before reserving against a new quota.
		if seconds <= 0 || seconds > int(runtime.Profile.TimeoutSeconds) || !monthStart(session.CreatedAt.Time).Equal(monthStart(now)) {
			return ErrQuotaExceeded
		}
		expiry := now.Add(time.Duration(seconds) * time.Second)
		committed, err := q.GetSandboxMonthlyCommittedSeconds(ctx, db.GetSandboxMonthlyCommittedSecondsParams{EnterpriseID: session.EnterpriseID, Month: pgtype.Date{Time: monthStart(now), Valid: true}})
		if err != nil {
			return err
		}
		extra := int64(math.Ceil(expiry.Sub(session.ExpiresAt.Time).Seconds()))
		if extra > 0 && (committed > quota.MonthlySessionSeconds || extra > quota.MonthlySessionSeconds-committed) {
			return ErrQuotaExceeded
		}
		_, err = runtime.Client.Renew(ctx, session.UpstreamSessionID, expiry)
		if err != nil {
			return err
		}
		_, err = q.UpdateSandboxSessionExpiry(ctx, db.UpdateSandboxSessionExpiryParams{ID: id, Status: "running", ExpiresAt: pgtype.Timestamptz{Time: expiry, Valid: true}})
		return err
	})
}
