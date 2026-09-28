//go:build m4e2e

package dashboard

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"time"
)

// Only the isolated deployment harness creates this table and opts its Worker
// in. No HTTP/tool endpoint or production configuration exposes a checkpoint.
type queryCheckpointKey struct{}

func withQueryCheckpoint(ctx context.Context, check func(context.Context, string) error) context.Context {
	if os.Getenv("ARGUS_E2E_QUERY_CHECKPOINTS") != "1" {
		return ctx
	}
	return context.WithValue(ctx, queryCheckpointKey{}, check)
}
func afterQueryTarget(ctx context.Context) error {
	if check, ok := ctx.Value(queryCheckpointKey{}).(func(context.Context, string) error); ok {
		return check(ctx, "fetched_target")
	}
	return nil
}
func (w queryWork) checkpoint(ctx context.Context, stage string) error {
	if os.Getenv("ARGUS_E2E_QUERY_CHECKPOINTS") != "1" {
		return nil
	}
	var released bool
	err := w.jobs.Runtime.Store.Pool.QueryRow(ctx, `UPDATE argus_e2e_dashboard_checkpoints SET job_id=$3,entered_at=coalesce(entered_at,now()),hits=hits+1 WHERE enterprise_id=$1 AND conversation_id=$2 AND stage=$4 AND (job_id IS NULL OR job_id=$3) RETURNING released`, w.job.EnterpriseID, w.job.ConversationID, w.job.ID, stage).Scan(&released)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for !released {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		err = w.jobs.Runtime.Store.Pool.QueryRow(ctx, `SELECT released FROM argus_e2e_dashboard_checkpoints WHERE enterprise_id=$1 AND conversation_id=$2 AND job_id=$3 AND stage=$4`, w.job.EnterpriseID, w.job.ConversationID, w.job.ID, stage).Scan(&released)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
