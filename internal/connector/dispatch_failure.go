package connector

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"math"
)

// This is a trusted server path. The Connector result state machine continues
// to forbid queued -> failed, so a client cannot terminate undispatched work.
func (service Service) rejectCommandBeforeDispatch(ctx context.Context, identity TrustedIdentity, epoch int64, id, code string) (db.ConnectorCommand, error) {
	var result db.ConnectorCommand
	if len(code) > 128 || !validRelayErrorCode(code) {
		return result, ErrCommandState
	}
	err := service.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockConnectorRuntime(ctx, db.LockConnectorRuntimeParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch}); err != nil {
			return ErrConnectorFenced
		}
		var err error
		result, err = q.RejectConnectorCommandBeforeDispatch(ctx, db.RejectConnectorCommandBeforeDispatchParams{CommandID: id, EnterpriseID: identity.EnterpriseID, ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch, ErrorCode: pgtype.Text{String: code, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommandState
		}
		if err != nil {
			return err
		}
		if result.CredentialLeaseID.Valid {
			if err = q.RevokeCredentialLease(ctx, db.RevokeCredentialLeaseParams{EnterpriseID: identity.EnterpriseID, ID: result.CredentialLeaseID.UUID}); err != nil {
				return err
			}
		}
		if result.CommandType == "collector_management" {
			var request connectorv1.CollectorManagementCommand
			if protojson.Unmarshal(result.Payload, &request) == nil && request.GetDesiredRevision() > 0 && request.GetDesiredRevision() <= math.MaxInt64 {
				collector, parseErr := uuid.Parse(request.GetCollectorId())
				if parseErr == nil {
					_, lookupErr := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: int64(request.GetDesiredRevision())})
					if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
						return lookupErr
					}
					if lookupErr == nil {
						if err = finalizeCollectorManagement(ctx, q, result, "failed", nil, code); err != nil {
							return err
						}
					}
				}
			}
		}
		if result.CommandType == "host_connection_probe" || result.CommandType == "kubernetes_connection_probe" {
			if test, parseErr := uuid.Parse(result.OperationRef); parseErr == nil {
				_, err = q.CompleteConnectionTest(ctx, db.CompleteConnectionTestParams{ID: test, EnterpriseID: identity.EnterpriseID, Status: "failed", Result: []byte("{}"), ErrorCode: pgtype.Text{String: code, Valid: true}})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
		}
		_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: identity.EnterpriseID, Valid: true}, ActorType: "system", ActorID: "connector-gateway:" + identity.ConnectorID.String(), Action: "connector.command.rejected_before_dispatch", ResourceType: "connector_command", ResourceID: result.ID.String(), Result: "failure", Details: map[string]any{"summary": "Undelivered " + result.CommandType, "reason_code": code, "connection_epoch": epoch}})
		return err
	})
	return result, err
}

// Unknown/dependency errors leave the command queued and close the stream for
// normal bounded reconnection. They are never relabelled as invalid schemas.
func dispatchPreparationFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ""
	case errors.Is(err, ErrCommandState):
		return "CONNECTOR_COMMAND_SCHEMA_INVALID"
	case errors.Is(err, secret.ErrInvalidLease), errors.Is(err, secret.ErrCredentialUnavailable):
		return "CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, authorization.ErrAccessDenied):
		return "CONNECTOR_COMMAND_AUTHORIZATION_DENIED"
	case errors.Is(err, resource.ErrActionInvalidated):
		return "CONNECTOR_ACTION_INVALIDATED"
	default:
		return ""
	}
}
