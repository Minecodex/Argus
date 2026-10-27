package directexecutor

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/collectorstate"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"math"
)

func (executor *Executor) SweepCollectorOperations(ctx context.Context) error {
	return executor.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		recovered, err := q.RecoverCollectorOperationsForProjection(ctx)
		if err != nil {
			return err
		}
		expired, err := q.ExpireCollectorOperationsForProjection(ctx)
		if err != nil {
			return err
		}
		for _, operation := range append(recovered, expired...) {
			var command connectorv1.CollectorManagementCommand
			if protojson.Unmarshal(operation.Plan, &command) != nil || command.GetCollectorId() != operation.CollectorID.String() || command.GetOperation() != operation.Operation || command.GetDesiredRevision() == 0 || command.GetDesiredRevision() > math.MaxInt64 {
				continue
			}
			current, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{ID: operation.CollectorID, EnterpriseID: operation.EnterpriseID})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if current.DesiredRevision != int64(command.GetDesiredRevision()) {
				continue
			}
			if operation.Operation != "uninstall" {
				_, err = q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: operation.CollectorID, Revision: int64(command.GetDesiredRevision())})
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
			}
			projected, err := collectorstate.Apply(ctx, q, operation.EnterpriseID, operation.CollectorID, int64(command.GetDesiredRevision()), operation.Operation, false, operation.ErrorCode.String)
			if err != nil {
				return err
			}
			if !projected {
				continue
			}
			if !collectorstate.OutcomeUnknown(operation.ErrorCode.String) {
				if _, err = q.RollbackCollectorClaimMigrations(ctx, db.RollbackCollectorClaimMigrationsParams{EnterpriseID: operation.EnterpriseID, CollectorID: operation.CollectorID}); err != nil {
					return err
				}
			}
			if err = collectorstate.RouteAfterFailure(ctx, q, operation.EnterpriseID, operation.CollectorID, operation.Operation, operation.ErrorCode.String); err != nil {
				return err
			}
		}
		return nil
	})
}
