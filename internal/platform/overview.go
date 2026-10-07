package platform

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Overview struct {
	SampledAt          time.Time
	Counts             db.GetPlatformOverviewCountsRow
	FromMonth, ToMonth string
	MonthlyUsage       []db.GetPlatformMonthlySandboxUsageRow
}

// Overview reads complete counts and bounded monthly aggregates in one snapshot.
// It never derives platform totals from a limited list of sessions or enterprises.
func (service EnterpriseService) Overview(ctx context.Context) (Overview, error) {
	at := time.Now().UTC()
	from, to := overviewPeriod(at)
	result := Overview{SampledAt: at, FromMonth: from.Format("2006-01"), ToMonth: to.Format("2006-01")}
	tx, err := service.Store.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	q := service.Store.Queries.WithTx(tx)
	if result.Counts, err = q.GetPlatformOverviewCounts(ctx); err != nil {
		return result, err
	}
	result.MonthlyUsage, err = q.GetPlatformMonthlySandboxUsage(ctx, db.GetPlatformMonthlySandboxUsageParams{
		FromMonth: pgtype.Date{Time: from, Valid: true}, ToMonth: pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func overviewPeriod(at time.Time) (time.Time, time.Time) {
	utc := at.UTC()
	month := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	return month.AddDate(0, -11, 0), month.AddDate(0, 1, 0)
}
