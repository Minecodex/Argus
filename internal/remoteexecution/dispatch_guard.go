package remoteexecution

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/proto"
)

func guardDispatch(ctx context.Context, q *db.Queries, identity db.RemoteExecutionIdentity, payload proto.Message) error {
	var request, session, generation string
	write := true
	var batch *connectorv1.RemoteExecutionBatch
	cleanup := false
	switch value := payload.(type) {
	case *connectorv1.RemoteExecutionBatch:
		request = value.RequestId
		session = value.SessionId
		generation = value.SessionGeneration
		batch = value
	case *connectorv1.RemoteExecutionControl:
		request = value.RequestId
		session = value.SessionId
		generation = value.SessionGeneration
		batch = value.Binding
		write = value.Operation == "open"
		cleanup = value.Operation == "reclaim" || value.Operation == "stop"
	case *connectorv1.RemoteExecutionFile:
		request = value.RequestId
		session = value.SessionId
		generation = value.SessionGeneration
		write = value.Operation == "write_input" || value.Operation == "finish_input"
	default:
		return Error("REMOTE_INPUT_INVALID")
	}
	requestID, err := uuid.Parse(request)
	if err != nil {
		return Error("REMOTE_BINDING_INVALID")
	}
	r, err := q.LockRemoteRequest(ctx, db.LockRemoteRequestParams{ID: requestID, EnterpriseID: identity.EnterpriseID})
	if err != nil {
		return Error("REMOTE_BINDING_INVALID")
	}
	if write && (r.Status != "running" || r.StopRequested) {
		return Error("REMOTE_STOPPED")
	}
	if !cleanup && (r.Status == "awaiting_confirmation" || r.Status == "superseded" || r.Status == "expired") {
		return Error("REMOTE_NOT_APPROVED")
	}
	user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: r.OwnerUserID, EnterpriseID: r.EnterpriseID})
	if !cleanup && (err != nil || user.Status != "active" || user.AuthorizationVersion != r.AuthorizationVersion) {
		return Error("REMOTE_AUTHORIZATION_CHANGED")
	}
	var spec Spec
	if json.Unmarshal(r.Spec, &spec) != nil || Digest(spec) != r.ContentHash {
		return Error("REMOTE_PLAN_INVALID")
	}
	var found *Step
	for n := range spec.Steps {
		step := &spec.Steps[n]
		if batch != nil && batch.Sequence != uint32(n+1) {
			continue
		}
		if step.SessionID.String() == session && step.Generation.String() == generation && step.Identity.ID == identity.ID {
			found = step
			if batch != nil && batch.Script == step.Script {
				break
			}
		}
	}
	if found == nil {
		return Error("REMOTE_BINDING_INVALID")
	}
	if !cleanup {
		permission := "remote_execution.read"
		if write {
			permission = "remote_execution.execute"
		}
		p, e := principalWithQueries(ctx, q, r.EnterpriseID, r.OwnerUserID, r.ConversationID, permission)
		if e != nil {
			return e
		}
		if e = authorizeTarget(ctx, q, p, found.Target); e != nil {
			return e
		}
		current, e := (Service{}).resolveIdentity(ctx, q, p, found.Target)
		if e != nil || current.ID != identity.ID || current.Version != found.Identity.Version {
			return Error("REMOTE_IDENTITY_CHANGED")
		}
	}
	if batch != nil && (batch.ApprovedContentHash != r.ContentHash || batch.Script != found.Script || batch.ScriptSha256 != hashBytes([]byte(found.Script)) || batch.IdentityBinding != identity.ID.String() || batch.Username != identity.Username || batch.IdentitySid != found.Identity.SID || batch.ResourceId != found.Target.ResourceID.String() || batch.ResourceType != found.Target.ResourceType || batch.Namespace != found.Target.Namespace || batch.Pod != found.Target.Pod || batch.PodUid != found.Target.PodUID || batch.Container != found.Target.Container || batch.Interpreter != found.Interpreter || batch.TimeoutSeconds != uint32(found.TimeoutSeconds) || batch.ActionRef != r.ActionRef) {
		return Error("REMOTE_BINDING_INVALID")
	}
	if write {
		target, e := resolveTarget(ctx, q, r.EnterpriseID, found.Target)
		if e != nil || target.ResourceVersion != found.Target.ResourceVersion {
			return Error("REMOTE_CONTEXT_CHANGED")
		}
		sessionID, _ := uuid.Parse(session)
		current, err := q.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: sessionID, EnterpriseID: identity.EnterpriseID})
		if err != nil || (current.Status != "new" && current.Status != "active") || current.Generation.String() != generation || !current.ActiveRequestID.Valid || current.ActiveRequestID.UUID != r.ID {
			return Error("REMOTE_CONTEXT_CHANGED")
		}
		if batch != nil && batch.ContextRevision != uint64(current.ContextRevision) {
			return Error("REMOTE_CONTEXT_CHANGED")
		}
		if batch != nil {
			steps, err := q.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID})
			if err != nil {
				return err
			}
			if len(steps) != len(spec.Steps) || batch.Sequence == 0 || int(batch.Sequence) > len(steps) {
				return Error("REMOTE_PLAN_INVALID")
			}
			for _, record := range steps {
				if record.Sequence < int32(batch.Sequence) && record.Status != "succeeded" {
					return Error("REMOTE_NOT_APPROVED")
				}
				if record.Sequence == int32(batch.Sequence) && (record.ID != found.ID || record.Status != "preparing" && record.Status != "running") {
					return Error("REMOTE_NOT_APPROVED")
				}
			}
		}
	}
	return nil
}
