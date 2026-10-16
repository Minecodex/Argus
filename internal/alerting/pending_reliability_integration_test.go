package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type failingActionDB struct {
	db.DBTX
	statement, code string
	injected        *bool
}

type actionErrorRow struct{ err error }

func (r actionErrorRow) Scan(...any) error { return r.err }

func (d failingActionDB) fail(sql string) error {
	if !*d.injected && strings.Contains(sql, d.statement) {
		*d.injected = true
		return &pgconn.PgError{Code: d.code, Message: "injected action transaction conflict"}
	}
	return nil
}

func (d failingActionDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := d.fail(sql); err != nil {
		return actionErrorRow{err}
	}
	return d.DBTX.QueryRow(ctx, sql, args...)
}

func (d failingActionDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := d.fail(sql); err != nil {
		return pgconn.CommandTag{}, err
	}
	return d.DBTX.Exec(ctx, sql, args...)
}

// Exercise real transactions, including rollback after token consumption.
// The injected DBTX error reproduces the SQLSTATE previously hidden by the
// PendingAction boundary; retries must never consume the token twice.
func TestPostgresPendingActionSQLStateSurvivesEveryExecutionStage(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		for _, stage := range []string{"GetPendingActionPlan", "GetPendingActionTokenForUpdate", "ConsumePendingActionToken", "MarkPendingActionExecutingM4"} {
			t.Run(code+"/"+stage, func(t *testing.T) {
				f := newAlertFixture(t)
				policy := f.createPolicy()
				p, _ := decodeSpec[PolicySpec](policy.Spec)
				p.Rules[0].Threshold = 90
				raw, _ := json.Marshal(p)
				pending, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: "policy", Operation: "update", ID: policy.ID, Name: policy.Name, ExpectedVersion: policy.Version, Spec: raw}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				extension := ActionExtension{}
				workflow := actionservice.Service{Store: f.s.Store, Idempotency: f.s.Actions.Idempotency, Resources: resource.Service{Store: f.s.Store, Actions: f.s.Actions, Extension: extension}}
				confirmed, err := workflow.Confirm(t.Context(), f.a.SubjectID.String(), uuid.NewString(), f.a.EnterpriseID, f.a.AuthorizationVersion, false, pending.ActionRef, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				injected, attempts := false, 0
				_, err = retryAlertWrite(t.Context(), func() (resource.ActionCommitResult, error) {
					attempts++
					tx, err := f.s.Store.Pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
					if err != nil {
						return resource.ActionCommitResult{}, err
					}
					defer tx.Rollback(t.Context())
					q := db.New(failingActionDB{DBTX: tx, statement: "-- name: " + stage, code: code, injected: &injected})
					result, err := f.s.Actions.ExecuteReady(t.Context(), q, confirmed.PendingAction, extension.RevalidateAction, extension.CommitAction)
					if err == nil {
						return result, tx.Commit(t.Context())
					}
					_ = tx.Rollback(t.Context())
					var pg *pgconn.PgError
					if !errors.As(err, &pg) || pg.Code != code || errors.Is(err, resource.ErrActionUnavailable) || errors.Is(err, resource.ErrActionInvalidated) {
						t.Fatalf("action %s stage %s hid SQLSTATE: %v", pending.ID, stage, err)
					}
					var state, token string
					var version int64
					if check := f.s.Store.Pool.QueryRow(t.Context(), "SELECT p.status,t.status,c.version FROM pending_actions p JOIN pending_action_tokens t ON t.pending_action_id=p.id JOIN alert_configurations c ON c.id=p.resource_id WHERE p.id=$1", pending.ID).Scan(&state, &token, &version); check != nil || state != "ready" || token != "active" || version != 1 {
						t.Fatalf("failed execution did not roll back: %s %s %d %v", state, token, version, check)
					}
					return result, err
				})
				if err != nil || !injected || attempts != 2 {
					t.Fatal("bounded retry failed", attempts, injected, err)
				}
				var state, token string
				var version int64
				if err = f.s.Store.Pool.QueryRow(t.Context(), "SELECT p.status,t.status,c.version FROM pending_actions p JOIN pending_action_tokens t ON t.pending_action_id=p.id JOIN alert_configurations c ON c.id=p.resource_id WHERE p.id=$1", pending.ID).Scan(&state, &token, &version); err != nil || state != "executing" || token != "consumed" || version != 2 {
					t.Fatal("retry did not commit exactly once", state, token, version, err)
				}
			})
		}
	}
}
