package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	Queue        = "remote_execution"
	ControlQueue = "remote_execution_control"
)

type Task struct {
	Operation    string          `json:"operation,omitempty"`
	IdentityID   uuid.UUID       `json:"identity_id,omitempty"`
	StepID       uuid.UUID       `json:"step_id,omitempty"`
	Binding      json.RawMessage `json:"binding,omitempty"`
	RequestID    uuid.UUID       `json:"request_id"`
	EnterpriseID uuid.UUID       `json:"enterprise_id"`
}

func (s Service) Handle(ctx context.Context, task runtime.Task) error {
	ctx = context.WithValue(ctx, transferAttemptKey{}, task.Attempt)
	var input Task
	if json.Unmarshal(task.Payload, &input) != nil || input.RequestID == uuid.Nil || input.EnterpriseID == uuid.Nil {
		return Error("REMOTE_INPUT_INVALID")
	}
	if task.Queue == ControlQueue && input.Operation != "stop" {
		return Error("REMOTE_INPUT_INVALID")
	}
	if input.Operation == "reclaim" {
		return s.handleReclamation(ctx, input)
	}
	if input.Operation == "stop" {
		return s.handleStop(ctx, input)
	}
	if input.Operation == "collect" {
		return s.handleLateCollection(ctx, input)
	}
	req, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: input.RequestID, EnterpriseID: input.EnterpriseID})
	if err != nil {
		return err
	}
	if req.Status != "ready" && req.Status != "running" && req.Status != "stopping" {
		return nil
	}
	var spec Spec
	if json.Unmarshal(req.Spec, &spec) != nil || Digest(spec) != req.ContentHash {
		return s.finish(ctx, req, "failed", "REMOTE_PLAN_INVALID")
	}
	if s.Transport == nil {
		return s.finish(ctx, req, "failed", "REMOTE_ROUTE_UNAVAILABLE")
	}
	if req.Status == "ready" {
		n, e := s.Store.Queries.StartRemoteRequest(ctx, db.StartRemoteRequestParams{ID: req.ID, EnterpriseID: req.EnterpriseID})
		if e != nil {
			return e
		}
		if n != 1 {
			return nil
		}
	}
	if err = s.reserve(ctx, req, spec); err != nil {
		if errorCode(err, "") == "REMOTE_CONTEXT_CHANGED" {
			return s.finish(ctx, req, "paused_for_recovery", "REMOTE_CONTEXT_CHANGED")
		}
		return err
	}
	steps, err := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: req.ID, EnterpriseID: req.EnterpriseID})
	if err != nil {
		return err
	}
	succeeded := 0
	for n, record := range steps {
		if record.Status == "succeeded" {
			succeeded++
			continue
		}
		if record.Status != "pending" && record.Status != "preparing" && record.Status != "running" && record.Status != "collecting" {
			return s.finish(ctx, req, record.Status, "REMOTE_STEP_INCOMPLETE")
		}
		current, e := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: req.ID, EnterpriseID: req.EnterpriseID})
		if e != nil {
			return e
		}
		if current.StopRequested && record.CommandID == "" {
			if _, e = s.Store.Queries.RecordRemoteStopReceipt(ctx, db.RecordRemoteStopReceiptParams{ID: req.ID, EnterpriseID: req.EnterpriseID, StopStatus: "confirmed"}); e != nil {
				return e
			}
			return s.finish(ctx, req, "cancelled", "REMOTE_STOPPED")
		}
		step := spec.Steps[n]
		result, e := s.runStep(ctx, req, spec, step, record, n+1)
		if e != nil {
			return e
		}
		current, e = s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: req.ID, EnterpriseID: req.EnterpriseID})
		if e != nil {
			return e
		}
		if current.StopRequested {
			if !result.Terminated && current.StopStatus != "confirmed" {
				return s.finish(ctx, req, "result_unknown", "REMOTE_STOP_UNKNOWN")
			}
			if current.StopStatus != "confirmed" {
				if _, e = s.Store.Queries.RecordRemoteStopReceipt(ctx, db.RecordRemoteStopReceiptParams{ID: req.ID, EnterpriseID: req.EnterpriseID, StopStatus: "confirmed"}); e != nil {
					return e
				}
			}
			return s.finish(ctx, req, "cancelled", "REMOTE_STOPPED")
		}
		if result.Status != "succeeded" {
			status := result.Status
			if status == "failed" && succeeded > 0 {
				status = "partial_failed"
			}
			return s.finish(ctx, req, status, result.ErrorCode)
		}
		succeeded++
	}
	return s.finish(ctx, req, "succeeded", "")
}
func batchBinding(r db.RemoteExecutionRequest, s Step, sequence int, revision int64) *connectorv1.RemoteExecutionBatch {
	return &connectorv1.RemoteExecutionBatch{RequestId: r.ID.String(), SessionId: s.SessionID.String(), SessionGeneration: s.Generation.String(), ContextRevision: uint64(revision), Sequence: uint32(sequence), ApprovedContentHash: r.ContentHash, Script: s.Script, ScriptSha256: hashBytes([]byte(s.Script)), Interpreter: s.Interpreter, Username: s.Identity.Username, IdentitySid: s.Identity.SID, IdentityBinding: s.Identity.ID.String(), TimeoutSeconds: uint32(s.TimeoutSeconds), MaxOutputBytes: MaxOutputBytes, ResourceId: s.Target.ResourceID.String(), ResourceType: s.Target.ResourceType, Namespace: s.Target.Namespace, Pod: s.Target.Pod, PodUid: s.Target.PodUID, Container: s.Target.Container, ActionRef: r.ActionRef}
}
func (s Service) reserve(ctx context.Context, r db.RemoteExecutionRequest, spec Spec) error {
	// Explicit, ordered session row locks serialize only this chat's contexts.
	// Serializable predicate conflicts must not turn another chat's execution
	// into a spurious context-loss event.
	ordered := append([]Step(nil), spec.Steps...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SessionID.String() < ordered[j].SessionID.String() })
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		seen := map[uuid.UUID]bool{}
		for _, step := range ordered {
			if seen[step.SessionID] {
				continue
			}
			seen[step.SessionID] = true
			session, e := q.LockRemoteSession(ctx, db.LockRemoteSessionParams{ID: step.SessionID, EnterpriseID: r.EnterpriseID})
			if e != nil {
				return e
			}
			if session.ActiveRequestID.Valid && session.ActiveRequestID.UUID == r.ID {
				continue
			}
			n, e := q.ReserveRemoteSession(ctx, db.ReserveRemoteSessionParams{ID: step.SessionID, EnterpriseID: r.EnterpriseID, ActiveRequestID: uuid.NullUUID{UUID: r.ID, Valid: true}, Generation: step.Generation, ContextRevision: step.ContextRevision})
			if e != nil {
				return e
			}
			if n != 1 {
				return Error("REMOTE_CONTEXT_CHANGED")
			}
			if e = q.TouchRemoteGeneration(ctx, db.TouchRemoteGenerationParams{SessionID: step.SessionID, Generation: step.Generation, EnterpriseID: r.EnterpriseID, Status: session.Status}); e != nil {
				return e
			}
		}
		return nil
	})
}
func (s Service) finish(ctx context.Context, r db.RemoteExecutionRequest, status, code string) error {
	return retryStateTx(ctx, s.Store, func(q *db.Queries) error {
		current, e := q.LockRemoteRequest(ctx, db.LockRemoteRequestParams{ID: r.ID, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return e
		}
		if current.Status != "ready" && current.Status != "running" && current.Status != "stopping" {
			return nil
		}
		if _, err := q.FinishRemoteRequest(ctx, db.FinishRemoteRequestParams{ID: r.ID, EnterpriseID: r.EnterpriseID, Status: status, ErrorCode: code}); err != nil {
			return err
		}
		if err := q.SkipRemoteSteps(ctx, db.SkipRemoteStepsParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID}); err != nil {
			return err
		}
		steps, e := q.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return e
		}
		for _, step := range steps {
			if step.Status == "preparing" && step.CommandID == "" {
				encoded, _ := json.Marshal(StepResult{Sequence: int(step.Sequence), Status: "cancelled", ErrorCode: code, OutputFiles: []File{}})
				if _, e = q.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: r.EnterpriseID, Status: "cancelled", Result: encoded}); e != nil {
					return e
				}
			}
		}
		if err := q.ReleaseRemoteRequestSessions(ctx, db.ReleaseRemoteRequestSessionsParams{EnterpriseID: r.EnterpriseID, ActiveRequestID: uuid.NullUUID{UUID: r.ID, Valid: true}}); err != nil {
			return err
		}
		if _, err := q.LockConversation(ctx, db.LockConversationParams{ID: r.ConversationID, EnterpriseID: r.EnterpriseID, OwnerUserID: r.OwnerUserID}); err != nil {
			return err
		}
		_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: r.EnterpriseID, ConversationID: r.ConversationID, RunID: uuid.NullUUID{UUID: r.RunID, Valid: true}, Type: "execution_update", ActorType: "service", Payload: map[string]any{"remote_execution_id": r.ID, "action_ref": r.ActionRef, "status": status, "error_code": code}, Classification: "internal"})
		return err
	})
}
func (s Service) watchStop(ctx context.Context, r db.RemoteExecutionRequest, step Step, identity db.RemoteExecutionIdentity, done <-chan struct{}, cancel context.CancelFunc) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-tick.C:
			current, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: r.ID, EnterpriseID: r.EnterpriseID})
			if err != nil {
				continue
			}
			invalid := s.validateRequest(ctx, s.Store.Queries, r, Spec{Steps: []Step{step}}, false) != nil
			if current.StopRequested || invalid {
				stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				raw, stopErr := s.Transport.Call(stopCtx, identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "stop"}, r.ID.String()+"/stop/"+step.ID.String(), 15*time.Second, nil)
				status := "unknown"
				var receipt connectorv1.RemoteExecutionControlResult
				if stopErr == nil && protojson.Unmarshal(raw, &receipt) == nil && receipt.SessionGeneration == step.Generation.String() && receipt.Status == "stopped" {
					status = "confirmed"
				}
				_, _ = s.Store.Queries.RecordRemoteStopReceipt(stopCtx, db.RecordRemoteStopReceiptParams{ID: r.ID, EnterpriseID: r.EnterpriseID, StopStatus: status})
				stopCancel()
				deadline := time.NewTimer(10 * time.Second)
				defer deadline.Stop()
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-deadline.C:
					cancel()
				}
				return
			}
		}
	}
}
func errorCode(err error, fallback string) string {
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return fallback
}
func timeValue(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
