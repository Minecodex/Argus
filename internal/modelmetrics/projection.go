package modelmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

const callsTable = "argus_internal_model_calls_v1"
const quotasTable = "argus_internal_model_quotas_v1"
const reservationsTable = "argus_internal_model_reservations_v1"

// These tables are deliberately absent from TenantTableRouter and Data Catalog.
// Only this package's fixed RPC can read them. DDL uses the existing schema role;
// the projector uses INSERT-only ClickHouse credentials.
func EnsureSchema(ctx context.Context, conn driver.Conn) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS argus_internal_model_reservations_v1 (enterprise_id UUID,id UUID,department_id UUID,user_id UUID,model_id UUID,month LowCardinality(String),version UInt64,payload String,projected_at DateTime64(6,'UTC') DEFAULT now64(6)) ENGINE=ReplacingMergeTree(version) ORDER BY (enterprise_id,id)`,
		`CREATE TABLE IF NOT EXISTS argus_internal_model_calls_v1 (enterprise_id UUID,id UUID,department_id UUID,user_id UUID,model_id UUID,completed_at DateTime64(6,'UTC'),version UInt64,payload String,projected_at DateTime64(6,'UTC') DEFAULT now64(6)) ENGINE=ReplacingMergeTree(version) ORDER BY (enterprise_id,id)`,
		`CREATE TABLE IF NOT EXISTS argus_internal_model_quotas_v1 (enterprise_id UUID,id UUID,subject_type LowCardinality(String),subject_id UUID,model_id UUID,effective_at DateTime64(6,'UTC'),version UInt64,payload String,projected_at DateTime64(6,'UTC') DEFAULT now64(6)) ENGINE=ReplacingMergeTree(version) ORDER BY (enterprise_id,id,version)`,
	} {
		if err := conn.Exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

type Projector struct {
	Store *postgres.Store
	Conn  driver.Conn
}

// A crash after ClickHouse INSERT but before PostgreSQL acknowledgement replays
// the same outbox version. ReplacingMergeTree + FINAL makes that replay idempotent.
func (p Projector) Batch(ctx context.Context) (int, error) {
	if p.Store == nil || p.Conn == nil {
		return 0, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tx, err := p.Store.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,kind,payload FROM model_metric_outbox WHERE delivered_at IS NULL ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 250`)
	if err != nil {
		return 0, err
	}
	type event struct {
		id      int64
		kind    string
		payload []byte
	}
	events := []event{}
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.id, &e.kind, &e.payload); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if !slices.Contains([]string{"call", "quota", "reservation"}, event.kind) {
			return 0, ErrInvalid
		}
	}
	for _, kind := range []string{"call", "quota", "reservation"} {
		query := `INSERT INTO ` + callsTable + ` (enterprise_id,id,department_id,user_id,model_id,completed_at,version,payload)`
		if kind == "quota" {
			query = `INSERT INTO ` + quotasTable + ` (enterprise_id,id,subject_type,subject_id,model_id,effective_at,version,payload)`
		}
		if kind == "reservation" {
			query = `INSERT INTO ` + reservationsTable + ` (enterprise_id,id,department_id,user_id,model_id,month,version,payload)`
		}
		selected := []event{}
		for _, e := range events {
			if e.kind == kind {
				selected = append(selected, e)
			}
		}
		if len(selected) == 0 {
			continue
		}
		batch, e := p.Conn.PrepareBatch(clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"async_insert": 0})), query)
		if e != nil {
			return 0, e
		}
		for _, event := range selected {
			if kind == "call" {
				var c Call
				if e = json.Unmarshal(event.payload, &c); e == nil {
					e = batch.Append(c.EnterpriseID, c.ID, c.DepartmentID, c.UserID, c.ModelID, c.CompletedAt, uint64(event.id), string(event.payload))
				}
			} else if kind == "reservation" {
				var r Reservation
				if e = json.Unmarshal(event.payload, &r); e == nil {
					e = batch.Append(r.EnterpriseID, r.ID, r.DepartmentID, r.UserID, r.ModelID, r.Month, uint64(event.id), string(event.payload))
				}
			} else {
				var q Quota
				if e = json.Unmarshal(event.payload, &q); e == nil {
					e = batch.Append(q.EnterpriseID, q.ID, q.SubjectType, q.SubjectID, q.ModelID, q.EffectiveAt, uint64(event.id), string(event.payload))
				}
			}
			if e != nil {
				_ = batch.Abort()
				return 0, e
			}
		}
		if e = batch.Send(); e != nil {
			return 0, e
		}
	}
	for _, e := range events {
		if _, err = tx.Exec(ctx, `UPDATE model_metric_outbox SET delivered_at=clock_timestamp() WHERE id=$1`, e.id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(events), nil
}

func (p Projector) Run(ctx context.Context, logger *slog.Logger) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n, err := p.Batch(ctx)
			delay := time.Second
			if err != nil {
				if logger != nil {
					logger.Warn("model metric projection delayed", "error", fmt.Sprint(err))
				}
				delay = 5 * time.Second
			} else if n == 250 {
				delay = time.Millisecond
			}
			timer.Reset(delay)
		}
	}
}
