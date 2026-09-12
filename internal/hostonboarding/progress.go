package hostonboarding

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var stages = []string{"queued", "probing", "transferring", "installing", "enrolling", "waiting_online", "completed"}
var ErrCancelled = errors.New("Host onboarding operation cancelled")

// Advance moves an active onboarding operation forward and writes a complete,
// ordered event trail. A path may advance several stages at once (manual
// bootstrap); intermediate stages are then recorded as started and succeeded.
func Advance(ctx context.Context, q *db.Queries, operationID, enterpriseID uuid.UUID, target string) error {
	operation, err := q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if operation.Status == "cancelled" {
		return ErrCancelled
	}
	if operation.Status != "queued" && operation.Status != "running" {
		return nil
	}
	currentIndex, targetIndex := slices.Index(stages, operation.Stage), slices.Index(stages, target)
	if currentIndex < 0 || targetIndex < 0 || targetIndex < currentIndex || target == "completed" {
		return errors.New("invalid Host onboarding stage transition")
	}
	if targetIndex == currentIndex {
		return nil
	}
	for index := currentIndex; index < targetIndex; index++ {
		if err = appendEvent(ctx, q, operation, stages[index], "succeeded", ""); err != nil {
			return err
		}
		operation, err = q.AdvanceHostOnboardingOperation(ctx, db.AdvanceHostOnboardingOperationParams{
			ID: operation.ID, EnterpriseID: operation.EnterpriseID, Stage: stages[index+1],
		})
		if err != nil {
			return err
		}
		if err = appendEvent(ctx, q, operation, stages[index+1], "started", ""); err != nil {
			return err
		}
	}
	return nil
}

func AdvanceByConnector(ctx context.Context, q *db.Queries, connectorID, enterpriseID uuid.UUID, target string) error {
	operation, err := q.GetActiveHostOnboardingOperationByConnector(ctx, db.GetActiveHostOnboardingOperationByConnectorParams{
		ConnectorID: connectorID, EnterpriseID: enterpriseID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return Advance(ctx, q, operation.ID, enterpriseID, target)
}

// RecordClaim closes queued and opens probing after a dispatcher has claimed
// the operation with SELECT FOR UPDATE SKIP LOCKED.
func RecordClaim(ctx context.Context, q *db.Queries, operation db.HostOnboardingOperation) error {
	if operation.Status != "running" || operation.Stage != "probing" {
		return errors.New("claimed Host onboarding operation is invalid")
	}
	if err := appendEvent(ctx, q, operation, "queued", "succeeded", ""); err != nil {
		return err
	}
	return appendEvent(ctx, q, operation, "probing", "started", "")
}

func Complete(ctx context.Context, q *db.Queries, operationID, enterpriseID uuid.UUID) error {
	operation, err := q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if operation.Status == "succeeded" {
		return nil
	}
	if operation.Status == "cancelled" {
		return ErrCancelled
	}
	if operation.Status != "queued" && operation.Status != "running" {
		return errors.New("Host onboarding operation cannot complete")
	}
	if err = Advance(ctx, q, operation.ID, operation.EnterpriseID, "waiting_online"); err != nil {
		return err
	}
	operation, err = q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, q, operation, "waiting_online", "succeeded", ""); err != nil {
		return err
	}
	if _, err = q.CompleteHostOnboardingOperation(ctx, db.CompleteHostOnboardingOperationParams{ID: operationID, EnterpriseID: enterpriseID}); err != nil {
		return err
	}
	operation.Stage = "completed"
	return appendEvent(ctx, q, operation, "completed", "succeeded", "")
}

func CompleteByConnector(ctx context.Context, q *db.Queries, connectorID, enterpriseID uuid.UUID) error {
	operation, err := q.GetActiveHostOnboardingOperationByConnector(ctx, db.GetActiveHostOnboardingOperationByConnectorParams{
		ConnectorID: connectorID, EnterpriseID: enterpriseID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return Complete(ctx, q, operation.ID, enterpriseID)
}

func RecordOutcome(ctx context.Context, q *db.Queries, operationID, enterpriseID uuid.UUID, status, errorCode string) error {
	if status != "failed" && status != "retrying" {
		return errors.New("invalid Host onboarding outcome")
	}
	operation, err := q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if operation.Status == "cancelled" {
		return ErrCancelled
	}
	return appendEvent(ctx, q, operation, operation.Stage, status, errorCode)
}

func appendEvent(ctx context.Context, q *db.Queries, operation db.HostOnboardingOperation, stage, status, errorCode string) error {
	current, err := q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
	if err != nil {
		return err
	}
	if current.Status == "cancelled" {
		return ErrCancelled
	}
	events, err := q.ListHostOnboardingOperationEvents(ctx, db.ListHostOnboardingOperationEventsParams{
		OperationID: operation.ID, EnterpriseID: operation.EnterpriseID,
	})
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Stage == stage && event.Status == status && event.ErrorCode.String == errorCode {
			return nil
		}
	}
	_, err = q.CreateHostOnboardingOperationEvent(ctx, db.CreateHostOnboardingOperationEventParams{
		ID: uuid.New(), OperationID: operation.ID, EnterpriseID: operation.EnterpriseID, Sequence: int64(len(events) + 1),
		Stage: stage, Status: status, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""},
	})
	return err
}
