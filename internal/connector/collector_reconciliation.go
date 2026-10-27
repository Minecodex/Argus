package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Only an authenticated current connection may settle an older uncertain
// Collector command. A saved typed receipt is evidence, never a new execution.
func (gateway Gateway) reconcileCollectorResult(ctx context.Context, identity TrustedIdentity, epoch int64, receipt *connectorv1.CommandResult, lookupErr error) error {
	if !errors.Is(lookupErr, pgx.ErrNoRows) || receipt == nil || receipt.GetConnectionEpoch() != uint64(epoch) || receipt.GetResultSchemaVersion() != "argus.connector_result/v1" ||
		receipt.GetTypedResult() == nil || len(receipt.GetResult()) != 0 || receipt.GetCommandId() == "" {
		return ErrCommandState
	}
	next := normalizeResultStatus(receipt.GetStatus())
	if next != "succeeded" && next != "failed" {
		return ErrCommandState
	}
	encoded, err := proto.Marshal(receipt.GetTypedResult())
	if err != nil || len(encoded) > MaxMessageBytes {
		return ErrCommandState
	}
	digest := sha256.Sum256(encoded)
	if !strings.EqualFold(receipt.GetResultHash(), hex.EncodeToString(digest[:])) {
		return ErrCommandState
	}
	value, err := marshalTypedResult(receipt.GetTypedResult())
	if err != nil || len(value) > MaxMessageBytes {
		return ErrCommandState
	}
	code := ""
	if receipt.GetError() != nil {
		code = receipt.GetError().GetCode()
	}
	if next == "succeeded" && code != "" {
		return ErrCommandState
	}
	return gateway.withCurrentRuntimeSession(ctx, identity, epoch, func(q *db.Queries) error {
		command, err := q.LockConnectorCommandForReconciliation(ctx, db.LockConnectorCommandForReconciliationParams{CommandID: receipt.GetCommandId(), EnterpriseID: identity.EnterpriseID, ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
		if err != nil || command.CommandType != "collector_management" || command.ConnectionEpoch >= epoch || !resultTypeAllowed(command.CommandType, receipt.GetTypedResult()) {
			return ErrCommandState
		}
		if next == "succeeded" && !validCollectorManagementResult(command, receipt.GetTypedResult()) {
			return ErrCommandState
		}
		if command.Status == "succeeded" || command.Status == "failed" {
			var stored, received connectorv1.CollectorManagementResult
			if command.Status == next && command.ErrorCode.String == code && protojson.Unmarshal(command.Result, &stored) == nil && receipt.GetTypedResult().UnmarshalTo(&received) == nil && proto.Equal(&stored, &received) {
				return nil
			}
			return ErrCommandState
		}
		if command.Status != "delivery_unknown" && command.Status != "result_unknown" {
			return ErrCommandState
		}
		hash := sha256.Sum256(value)
		_, err = q.TransitionConnectorCommand(ctx, db.TransitionConnectorCommandParams{CommandID: command.CommandID, ConnectorID: command.ConnectorID, ConnectionEpoch: command.ConnectionEpoch, ExpectedStatus: command.Status, Status: next, Result: value, ResultHash: hash[:], ErrorCode: pgtype.Text{String: code, Valid: code != ""}})
		if err != nil {
			return err
		}
		return finalizeCollectorManagement(ctx, q, command, next, value, code)
	})
}

func (gateway Gateway) sendCollectorReconciliation(stream connectorv1.ConnectorControlService_ConnectServer, identity TrustedIdentity, epoch int64, sequence *uint64) error {
	commands, err := gateway.Service.Store.Queries.ListUncertainCollectorCommands(stream.Context(), db.ListUncertainCollectorCommandsParams{ConnectorID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID, Limit: 64})
	if err != nil || len(commands) == 0 {
		return err
	}
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		ids = append(ids, command.CommandID)
	}
	if err = stream.Send(&connectorv1.ConnectResponse{Sequence: *sequence, Frame: &connectorv1.ConnectResponse_CommandReconcileRequest{CommandReconcileRequest: &connectorv1.CommandReconcileRequest{CommandIds: ids, ConnectionEpoch: uint64(epoch)}}}); err != nil {
		return err
	}
	*sequence++
	return nil
}
