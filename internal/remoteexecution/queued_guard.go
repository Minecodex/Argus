package remoteexecution

import (
	"context"
	"crypto/sha256"
	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Called inside the Gateway's dispatch transaction. The request lock orders a
// stop/revocation check before the durable transition to an in-flight command.
func ValidateQueuedCommand(ctx context.Context, q *db.Queries, command db.ConnectorCommand) error {
	canonical, hashErr := resource.CanonicalJSON(command.Payload)
	sum := sha256.Sum256(canonical)
	if hashErr != nil || string(sum[:]) != string(command.PayloadHash) {
		return Error("REMOTE_BINDING_INVALID")
	}
	var payload proto.Message
	var sessionID string
	switch command.CommandType {
	case "remote_execution_batch":
		v := &connectorv1.RemoteExecutionBatch{}
		payload = v
		if protojson.Unmarshal(command.Payload, v) != nil {
			return Error("REMOTE_BINDING_INVALID")
		}
		sessionID = v.SessionId
	case "remote_execution_control":
		v := &connectorv1.RemoteExecutionControl{}
		payload = v
		if protojson.Unmarshal(command.Payload, v) != nil {
			return Error("REMOTE_BINDING_INVALID")
		}
		sessionID = v.SessionId
	case "remote_execution_file":
		v := &connectorv1.RemoteExecutionFile{}
		payload = v
		if protojson.Unmarshal(command.Payload, v) != nil {
			return Error("REMOTE_BINDING_INVALID")
		}
		sessionID = v.SessionId
	default:
		return Error("REMOTE_BINDING_INVALID")
	}
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return Error("REMOTE_BINDING_INVALID")
	}
	session, err := q.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: id, EnterpriseID: command.EnterpriseID})
	if err != nil {
		return Error("REMOTE_BINDING_INVALID")
	}
	identity, err := q.GetRemoteIdentityForControl(ctx, db.GetRemoteIdentityForControlParams{ID: session.IdentityID, EnterpriseID: command.EnterpriseID})
	if err != nil {
		return Error("REMOTE_IDENTITY_UNAVAILABLE")
	}
	if !identity.ConnectorID.Valid || identity.ConnectorID.UUID != command.ConnectorID {
		return Error("REMOTE_BINDING_INVALID")
	}
	return guardDispatch(ctx, q, identity, payload)
}
