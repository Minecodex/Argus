package remoteexecution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type Workspace interface {
	ValidateRemoteInputs(context.Context, toolruntime.Principal, []File) error
	FreezeRemoteInputs(context.Context, toolruntime.Principal, uuid.UUID, uuid.UUID) ([]File, error)
	ImportRemoteOutput(context.Context, toolruntime.Principal, OutputArtifact, io.Reader) (db.WorkspaceFile, error)
}
type ObjectStore interface {
	ReadFile(context.Context, string, int64, int64) (io.ReadCloser, error)
	PutFile(context.Context, string, io.Reader, int64, string) error
}
type Service struct {
	Store       *postgres.Store
	Actions     resource.PendingActionService
	Next        resource.ActionExtension
	Files       Workspace
	Objects     ObjectStore
	Transport   Transport
	IdleTimeout time.Duration
}

func (s Service) principal(ctx context.Context, enterprise, user, convo uuid.UUID, required ...string) (toolruntime.Principal, error) {
	permission := "remote_execution.execute"
	if len(required) > 0 {
		permission = required[0]
	}
	return principalWithQueries(ctx, s.Store.Queries, enterprise, user, convo, permission)
}
func authorizeTarget(ctx context.Context, q *db.Queries, p toolruntime.Principal, t Target) error {
	ids, err := q.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{UserID: p.UserID, EnterpriseID: p.EnterpriseID, ResourceType: t.ResourceType})
	if err != nil {
		return err
	}
	if !slices.Contains(ids, t.ResourceID) {
		return Error("REMOTE_RESOURCE_FORBIDDEN")
	}
	return nil
}

func (s Service) Prepare(ctx context.Context, call toolruntime.Invocation, kind string, input Input) (View, error) {
	return s.prepare(ctx, call, kind, input, nil)
}
func (s Service) prepare(ctx context.Context, call toolruntime.Invocation, kind string, input Input, approved *approvedCommand) (View, error) {
	if err := input.Validate(kind); err != nil {
		return View{}, err
	}
	p, err := s.principal(ctx, call.Principal.EnterpriseID, call.Principal.UserID, call.Principal.ConversationID)
	if err != nil {
		return View{}, err
	}
	if call.ID == uuid.Nil || call.RunID == uuid.Nil {
		return View{}, Error("REMOTE_INPUT_INVALID")
	}
	if kind == "plan" {
		parent, e := s.Store.Queries.GetRemoteFeedbackParent(ctx, db.GetRemoteFeedbackParentParams{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: call.RunID, Valid: true}, UserID: p.UserID, ToolCallID: call.ID})
		if e == nil {
			if input.SupersedesID != nil && *input.SupersedesID != parent {
				return View{}, Error("REMOTE_PLAN_INVALID")
			}
			input.SupersedesID = &parent
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return View{}, e
		}
	}
	if old, e := s.Store.Queries.GetRemoteRequestByToolCall(ctx, db.GetRemoteRequestByToolCallParams{ToolCallID: call.ID, EnterpriseID: p.EnterpriseID}); e == nil {
		if old.OwnerUserID != p.UserID || old.ConversationID != p.ConversationID || old.RunID != call.RunID {
			return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
		}
		var previous Spec
		if json.Unmarshal(old.Spec, &previous) != nil || previous.Kind != kind || Digest(editableInput(previous)) != Digest(editableInput(Spec{Title: input.Title, Description: input.Description, Steps: input.Steps, SupersedesID: input.SupersedesID, RecoveryOfID: input.RecoveryOfID})) {
			return View{}, Error("REMOTE_COMMAND_CONFLICT")
		}
		for _, step := range previous.Steps {
			if err = authorizeTarget(ctx, s.Store.Queries, p, step.Target); err != nil {
				return View{}, err
			}
		}
		if len(previous.Inputs) > 0 && (s.Files == nil || s.Files.ValidateRemoteInputs(ctx, p, previous.Inputs) != nil) {
			return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
		}
		return s.view(ctx, old)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return View{}, e
	}
	run, err := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: call.RunID, EnterpriseID: p.EnterpriseID})
	if err != nil || run.ConversationID != p.ConversationID || run.ActorUserID != p.UserID {
		return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	id := uuid.NewSHA1(call.ID, []byte("argus.remote_execution/v1"))
	spec := Spec{SchemaVersion: SchemaVersion, ID: id, Kind: kind, ConversationID: p.ConversationID, RunID: run.ID, Title: input.Title, Description: input.Description, Steps: input.Steps, Inputs: []File{}, SupersedesID: input.SupersedesID, RecoveryOfID: input.RecoveryOfID}
	revision := int32(1)
	for _, previous := range []*uuid.UUID{spec.SupersedesID, spec.RecoveryOfID} {
		if previous != nil {
			old, e := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: *previous, EnterpriseID: p.EnterpriseID})
			if e != nil || old.OwnerUserID != p.UserID || old.ConversationID != p.ConversationID {
				return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
			}
			if old.Status == "running" || old.Status == "stopping" || old.Status == "ready" {
				return View{}, Error("REMOTE_SESSION_BUSY")
			}
			revision = max(revision, old.Revision+1)
		}
	}
	if approved == nil {
		spec, err = s.freezeSpec(ctx, call, p, spec)
		if err != nil {
			return View{}, err
		}
	} else {
		spec = approved.Spec
	}
	snapshot := resourceSnapshot(spec)
	prepareInput := resource.PrepareActionInput{ActionType: "remote_execution.execute", Title: spec.Title, Summary: spec.Description, Risk: "write", ResourceType: "remote_execution", ResourceID: uuid.NullUUID{UUID: id, Valid: true}, AuthorizationVersion: p.AuthorizationVersion, Preview: spec, Diff: []map[string]string{}, ImmutablePlan: spec, ResourceScopeSnapshot: snapshot, CommitHandler: "remote_execution.commit", RunID: uuid.NullUUID{UUID: call.RunID, Valid: true}}
	raw, _ := json.Marshal(spec)
	if len(raw) > 896<<10 {
		return View{}, Error("REMOTE_PLAN_TOO_LARGE")
	}
	var req db.RemoteExecutionRequest
	createWithQueries := func(q *db.Queries) error {
		action, e := s.Actions.PrepareWithQueries(ctx, q, resource.ActionSubject{ID: p.UserID.String(), Type: "user", AuthorizationVersion: p.AuthorizationVersion, ConfirmationRequired: true}, p.EnterpriseID, prepareInput)
		if e != nil {
			return e
		}
		if approved != nil {
			if e = q.BindActionPlanCommand(ctx, db.BindActionPlanCommandParams{ID: action.ID, EnterpriseID: p.EnterpriseID, ApprovedPlanCommandID: uuid.NullUUID{UUID: approved.Grant.ID, Valid: true}}); e != nil {
				return e
			}
		}
		req, err = q.CreateRemoteRequest(ctx, db.CreateRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID, RunID: call.RunID, ToolCallID: call.ID, Kind: kind, Revision: revision, SupersedesID: nullID(spec.SupersedesID), RecoveryOfID: nullID(spec.RecoveryOfID), ActionRef: action.ActionRef, ContentHash: Digest(spec), Spec: raw, AuthorizationVersion: p.AuthorizationVersion})
		if err != nil {
			return err
		}
		for n, step := range spec.Steps {
			if _, err = q.CreateRemoteStep(ctx, db.CreateRemoteStepParams{ID: step.ID, EnterpriseID: p.EnterpriseID, RequestID: id, SessionID: step.SessionID, Generation: step.Generation, Sequence: int32(n + 1)}); err != nil {
				return err
			}
		}
		for _, f := range spec.Inputs {
			inputOwner := id
			if approved != nil {
				inputOwner = approved.Grant.PlanID
			}
			if _, err = q.CreateRemoteFile(ctx, db.CreateRemoteFileParams{ID: f.ID, EnterpriseID: p.EnterpriseID, RequestID: id, Direction: "input", Path: f.Path, ByteSize: f.Size, ContentHash: f.Hash, ObjectKey: inputObjectKey(p.EnterpriseID, inputOwner, f.Path, f.Hash), Status: "sealed", SourceRef: f.SourceRef}); err != nil {
				return err
			}
		}
		if spec.SupersedesID != nil {
			if err = s.supersede(ctx, q, p, *spec.SupersedesID); err != nil {
				return err
			}
		}
		return nil
	}
	req, err = postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", p.UserID.String(), "remote_execution.prepare", call.ID.String(), spec, 201, func(q *db.Queries) (db.RemoteExecutionRequest, error) {
		if approved != nil {
			g, e := s.lockPlanCommand(ctx, q, call, approved)
			if e != nil {
				return db.RemoteExecutionRequest{}, e
			}
			if g.UsedRequestID.Valid {
				return q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: g.UsedRequestID.UUID, EnterpriseID: p.EnterpriseID})
			}
		}
		if e := createWithQueries(q); e != nil {
			return db.RemoteExecutionRequest{}, e
		}
		if approved != nil {
			n, e := q.UsePlanCommandAuthorization(ctx, db.UsePlanCommandAuthorizationParams{ID: approved.Grant.ID, EnterpriseID: p.EnterpriseID, UsedRequestID: uuid.NullUUID{UUID: req.ID, Valid: true}})
			if e != nil {
				return db.RemoteExecutionRequest{}, e
			}
			if n != 1 {
				return db.RemoteExecutionRequest{}, Error("REMOTE_COMMAND_CONFLICT")
			}
		}
		return req, nil
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, req)
}
func nullID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
func inputObjectKey(enterprise, request uuid.UUID, path, digest string) string {
	return "remote/" + enterprise.String() + "/" + request.String() + "/input/" + digest + "/" + path
}
func InputObjectKey(enterprise, request uuid.UUID, path, digest string) string {
	return inputObjectKey(enterprise, request, path, digest)
}
func resourceSnapshot(spec Spec) map[string]any {
	targets := []map[string]any{}
	for _, step := range spec.Steps {
		var target map[string]any
		encoded, _ := json.Marshal(step.Target)
		_ = json.Unmarshal(encoded, &target)
		targets = append(targets, target)
	}
	return map[string]any{"targets": targets}
}

func (s Service) RevalidateAction(ctx context.Context, q *db.Queries, a db.PendingAction, raw json.RawMessage) ([]byte, error) {
	if a.ActionType != "remote_execution.execute" {
		if s.Next != nil {
			return s.Next.RevalidateAction(ctx, q, a, raw)
		}
		return nil, resource.ErrActionInvalidated
	}
	var spec Spec
	if json.Unmarshal(raw, &spec) != nil {
		return nil, resource.ErrActionInvalidated
	}
	request, err := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: spec.ID, EnterpriseID: a.EnterpriseID})
	if err != nil || request.ContentHash != Digest(spec) || request.Status != "awaiting_confirmation" || request.StopRequested || request.OwnerUserID != a.CreatorSubjectID {
		return nil, resource.ErrActionInvalidated
	}
	if err = s.validateRequest(ctx, q, request, spec, true); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(resourceSnapshot(spec))
	b, err = resource.CanonicalJSON(b)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(b)
	return hash[:], nil
}
func (s Service) CommitAction(ctx context.Context, q *db.Queries, a db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	if a.ActionType != "remote_execution.execute" {
		if s.Next != nil {
			return s.Next.CommitAction(ctx, q, a, raw)
		}
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	var spec Spec
	if json.Unmarshal(raw, &spec) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	n, err := q.ReadyRemoteRequest(ctx, db.ReadyRemoteRequestParams{ID: spec.ID, EnterpriseID: a.EnterpriseID})
	if err != nil || n != 1 {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	payload, _ := json.Marshal(Task{RequestID: spec.ID, EnterpriseID: a.EnterpriseID})
	_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: a.EnterpriseID, Valid: true}, Queue: Queue, RunID: a.RunID, Payload: payload, MaxAttempts: 3, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	return resource.ActionCommitResult{ResourceType: "remote_execution", ResourceID: spec.ID, Summary: "Remote execution approved and queued", RemoteExecutionID: uuid.NullUUID{UUID: spec.ID, Valid: true}}, err
}
func (s Service) validateRequest(ctx context.Context, q *db.Queries, r db.RemoteExecutionRequest, spec Spec, contextCheck bool) error {
	p, err := principalWithQueries(ctx, q, r.EnterpriseID, r.OwnerUserID, r.ConversationID, "remote_execution.execute")
	if err != nil || p.AuthorizationVersion != r.AuthorizationVersion {
		return resource.ErrActionInvalidated
	}
	if len(spec.Inputs) > 0 && (s.Files == nil || s.Files.ValidateRemoteInputs(ctx, p, spec.Inputs) != nil) {
		return resource.ErrActionInvalidated
	}
	for _, step := range spec.Steps {
		if err = authorizeTarget(ctx, q, p, step.Target); err != nil {
			return resource.ErrActionInvalidated
		}
		target, e := resolveTarget(ctx, q, p.EnterpriseID, step.Target)
		if e != nil || target.ResourceVersion != step.Target.ResourceVersion {
			return resource.ErrActionInvalidated
		}
		identity, err := s.resolveIdentity(ctx, q, p, step.Target)
		if err != nil || identity.ID != step.Identity.ID || identity.Version != step.Identity.Version {
			return resource.ErrActionInvalidated
		}
		session, err := q.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: r.EnterpriseID})
		if err != nil || session.Generation != step.Generation || session.OwnerUserID != r.OwnerUserID || session.ConversationID != r.ConversationID || (session.Status != "active" && session.Status != "new") {
			return resource.ErrActionInvalidated
		}
		if contextCheck && (session.ContextRevision != step.ContextRevision || session.ActiveRequestID.Valid || !observedContextMatches(session, step)) {
			return resource.ErrActionInvalidated
		}
	}
	return nil
}

func observedContextMatches(session db.RemoteExecutionSession, step Step) bool {
	return (step.KnownDirectory == "" || step.KnownDirectory == session.KnownDirectory) && (step.InterpreterVersion == "" || step.InterpreterVersion == session.InterpreterVersion)
}
