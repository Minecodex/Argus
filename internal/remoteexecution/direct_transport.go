package remoteexecution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"time"
)

func wireSession(payload proto.Message) (uuid.UUID, uuid.UUID, error) {
	var id, generation string
	switch p := payload.(type) {
	case *connectorv1.RemoteExecutionBatch:
		id, generation = p.SessionId, p.SessionGeneration
	case *connectorv1.RemoteExecutionControl:
		id, generation = p.SessionId, p.SessionGeneration
	case *connectorv1.RemoteExecutionFile:
		id, generation = p.SessionId, p.SessionGeneration
	default:
		return uuid.Nil, uuid.Nil, Error("REMOTE_BINDING_INVALID")
	}
	s, e := uuid.Parse(id)
	g, e2 := uuid.Parse(generation)
	if e != nil || e2 != nil {
		return uuid.Nil, uuid.Nil, Error("REMOTE_BINDING_INVALID")
	}
	return s, g, nil
}
func (t ConnectorTransport) callDirect(ctx context.Context, identity db.RemoteExecutionIdentity, kind string, payload proto.Message, key string, timeout time.Duration, queued func(context.Context, *db.Queries, string) error) (json.RawMessage, error) {
	session, generation, err := wireSession(payload)
	if err != nil {
		return nil, err
	}
	encoded, err := protojson.Marshal(payload)
	if err != nil || len(encoded) > 1<<20 {
		return nil, Error("REMOTE_INPUT_INVALID")
	}
	encoded, err = resource.CanonicalJSON(encoded)
	if err != nil {
		return nil, err
	}
	id := uuid.NewSHA1(identity.ID, []byte(key))
	hash := sha256.Sum256(encoded)
	commandID := "direct_remote_" + id.String()
	var command db.RemoteExecutionDirectCommand
	err = retryStateTx(ctx, t.Store, func(q *db.Queries) error {
		existing, e := q.GetRemoteDirectCommand(ctx, db.GetRemoteDirectCommandParams{ID: id, EnterpriseID: identity.EnterpriseID})
		if e == nil {
			if string(existing.PayloadHash) != string(hash[:]) || existing.CommandType != kind {
				return Error("REMOTE_COMMAND_CONFLICT")
			}
			command = existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = guardDispatch(ctx, q, identity, payload); e != nil {
			return e
		}
		command, e = q.CreateRemoteDirectCommand(ctx, db.CreateRemoteDirectCommandParams{ID: id, CommandID: commandID, EnterpriseID: identity.EnterpriseID, IdentityID: identity.ID, SessionID: session, Generation: generation, CommandType: kind, Payload: encoded, PayloadHash: hash[:], ExpiresAt: timeValue(time.Now().Add(timeout))})
		if e != nil {
			return e
		}
		if queued != nil {
			return queued(ctx, q, commandID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		switch command.Status {
		case "succeeded":
			return command.Result, nil
		case "failed":
			code := command.ErrorCode
			if code == "" {
				code = "REMOTE_COMMAND_FAILED"
			}
			if command.DispatchRejected {
				return command.Result, unsentCommandError{Error(code)}
			}
			return command.Result, Error(code)
		case "result_unknown":
			return command.Result, Error("REMOTE_RESULT_UNKNOWN")
		}
		select {
		case <-ctx.Done():
			return nil, Error("REMOTE_RESULT_UNKNOWN")
		case <-deadline.C:
			return nil, Error("REMOTE_RESULT_UNKNOWN")
		case <-tick.C:
			command, err = t.Store.Queries.GetRemoteDirectCommand(ctx, db.GetRemoteDirectCommandParams{ID: id, EnterpriseID: identity.EnterpriseID})
			if err != nil {
				return nil, err
			}
		}
	}
}

func ValidateDirectCommand(ctx context.Context, q *db.Queries, c db.RemoteExecutionDirectCommand, payload proto.Message) error {
	session, generation, e := wireSession(payload)
	if e != nil || session != c.SessionID || generation != c.Generation {
		return Error("REMOTE_BINDING_INVALID")
	}
	identity, err := q.GetRemoteIdentityForControl(ctx, db.GetRemoteIdentityForControlParams{ID: c.IdentityID, EnterpriseID: c.EnterpriseID})
	if err != nil || identity.ConnectorID.Valid || identity.Platform != "kubernetes" {
		return Error("REMOTE_IDENTITY_UNAVAILABLE")
	}
	return guardDispatch(ctx, q, identity, payload)
}
