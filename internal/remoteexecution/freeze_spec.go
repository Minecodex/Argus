package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"time"
)

// Resolve identities, shared context and sealed inputs without dispatching commands.
func (s Service) freezeSpec(ctx context.Context, call toolruntime.Invocation, p toolruntime.Principal, spec Spec) (Spec, error) {
	id := spec.ID
	var err error
	for n := range spec.Steps {
		step := &spec.Steps[n]
		if err := authorizeTarget(ctx, s.Store.Queries, p, step.Target); err != nil {
			return Spec{}, err
		}
		step.Target, err = resolveTarget(ctx, s.Store.Queries, p.EnterpriseID, step.Target)
		if err != nil {
			return Spec{}, err
		}
		identity, err := s.resolveIdentity(ctx, s.Store.Queries, p, step.Target)
		if err != nil {
			return Spec{}, err
		}
		if identity.Platform == "windows" && step.Interpreter != "powershell" || identity.Platform == "linux" && step.Interpreter != "bash" {
			return Spec{}, Error("REMOTE_INTERPRETER_UNAVAILABLE")
		}
		scope := targetScope(step.Target)
		targetJSON, _ := json.Marshal(scope)
		session, err := s.Store.Queries.CreateRemoteSession(ctx, db.CreateRemoteSessionParams{ID: uuid.NewSHA1(p.ConversationID, []byte(fmt.Sprintf("%s/%d/%s/%s", identity.ID, identity.Version, step.Interpreter, Digest(scope)))), EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID, ResourceType: step.Target.ResourceType, ResourceID: step.Target.ResourceID, IdentityID: identity.ID, IdentityVersion: identity.Version, Interpreter: step.Interpreter, Target: targetJSON, TargetHash: Digest(scope), Generation: uuid.New()})
		if err != nil {
			return Spec{}, err
		}
		if err = s.Store.Queries.EnsureRemoteGeneration(ctx, db.EnsureRemoteGenerationParams{SessionID: session.ID, Generation: session.Generation, EnterpriseID: p.EnterpriseID, Status: session.Status, LastActiveAt: session.LastActiveAt}); err != nil {
			return Spec{}, err
		}
		if session.ActiveRequestID.Valid {
			return Spec{}, Error("REMOTE_SESSION_BUSY")
		}
		if session.Status == "lost" || session.Status == "untrusted" || session.Status == "reclaimed" {
			if spec.RecoveryOfID == nil {
				// A preview may expire before any remote operation ever starts.
				// A new preview can create a fresh context, invalidating old grants.
				fresh, e := s.Store.Queries.ResetUnusedRemoteSession(ctx, db.ResetUnusedRemoteSessionParams{ID: session.ID, EnterpriseID: p.EnterpriseID, Generation: uuid.New()})
				if errors.Is(e, pgx.ErrNoRows) {
					return Spec{}, s.recoveryRequired(ctx, p, session)
				}
				if e != nil {
					return Spec{}, e
				}
				session = fresh
			} else {
				session, err = s.Store.Queries.ResetLostRemoteSession(ctx, db.ResetLostRemoteSessionParams{ID: session.ID, EnterpriseID: p.EnterpriseID, Generation: uuid.New()})
				if err != nil {
					return Spec{}, err
				}
			}
			if err = s.Store.Queries.EnsureRemoteGeneration(ctx, db.EnsureRemoteGenerationParams{SessionID: session.ID, Generation: session.Generation, EnterpriseID: p.EnterpriseID, Status: session.Status, LastActiveAt: session.LastActiveAt}); err != nil {
				return Spec{}, err
			}
		}
		step.ID = uuid.NewSHA1(id, []byte(time.Duration(n+1).String()))
		step.SessionID = session.ID
		step.Generation = session.Generation
		step.ContextRevision = session.ContextRevision
		step.KnownDirectory, step.InterpreterVersion = session.KnownDirectory, session.InterpreterVersion
		step.PreviousRequestID, step.PreviousSequence = nil, 0
		previous, e := s.Store.Queries.GetRemoteContextPredecessor(ctx, db.GetRemoteContextPredecessorParams{SessionID: session.ID, Generation: session.Generation, EnterpriseID: p.EnterpriseID})
		if e == nil {
			priorID := previous.RequestID
			step.PreviousRequestID = &priorID
			step.PreviousSequence = int(previous.Sequence)
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return Spec{}, e
		}
		step.Identity = IdentityView{ID: identity.ID, Username: identity.Username, SID: identity.IdentitySid, Version: identity.Version, Platform: identity.Platform}
	}
	if s.Files != nil {
		spec.Inputs, err = s.Files.FreezeRemoteInputs(ctx, p, call.RunID, id)
		if err != nil {
			return Spec{}, err
		}
	}
	return spec, nil
}
