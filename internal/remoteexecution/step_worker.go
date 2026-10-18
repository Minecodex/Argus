package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s Service) runStep(ctx context.Context, req db.RemoteExecutionRequest, spec Spec, step Step, record db.RemoteExecutionStep, sequence int) (StepResult, error) {
	identity, err := s.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: step.Identity.ID, EnterpriseID: req.EnterpriseID})
	if err != nil {
		return StepResult{}, err
	}
	if record.Status == "collecting" {
		var known StepResult
		if json.Unmarshal(record.Result, &known) != nil {
			return StepResult{}, Error("REMOTE_PLAN_INVALID")
		}
		return s.collectKnownStep(ctx, req, step, identity, known)
	}
	if record.CommandID == "" {
		if err = s.validateRequest(ctx, s.Store.Queries, req, spec, false); err != nil {
			return s.failUnsentStep(ctx, req, step, sequence, "failed", "REMOTE_AUTHORIZATION_CHANGED")
		}
	}
	session, err := s.Store.Queries.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: req.EnterpriseID})
	if err != nil {
		return StepResult{}, err
	}
	binding := batchBinding(req, step, sequence, session.ContextRevision)
	if record.Status == "pending" {
		if _, err = s.Store.Queries.StartRemoteStep(ctx, db.StartRemoteStepParams{ID: record.ID, EnterpriseID: req.EnterpriseID}); err != nil {
			return StepResult{}, err
		}
	}
	if record.CommandID == "" {
		if err = s.prepareFiles(ctx, req, step, identity, binding); err != nil {
			code := errorCode(err, "REMOTE_TRANSFER_FAILED")
			logPreparationFailure(ctx, req.ID, step.ID, code, err)
			if code == "REMOTE_CONTEXT_LOST" || code == "REMOTE_CONTEXT_CHANGED" || code == "REMOTE_RESULT_UNKNOWN" {
				if _, err = s.Store.Queries.MarkRemoteSessionLost(ctx, db.MarkRemoteSessionLostParams{ID: step.SessionID, EnterpriseID: req.EnterpriseID, Generation: step.Generation}); err != nil {
					return StepResult{}, err
				}
				return s.failUnsentStep(ctx, req, step, sequence, "paused_for_recovery", code)
			}
			return s.failUnsentStep(ctx, req, step, sequence, "failed", code)
		}
	}
	executeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go s.watchStop(executeCtx, req, step, identity, done, cancel)
	raw, callErr := s.Transport.Call(executeCtx, identity, "remote_execution_batch", binding, req.ID.String()+"/batch/"+step.ID.String(), time.Duration(step.TimeoutSeconds+30)*time.Second, func(ctx context.Context, q *db.Queries, id string) error {
		n, e := q.DispatchRemoteStep(ctx, db.DispatchRemoteStepParams{ID: step.ID, EnterpriseID: req.EnterpriseID, CommandID: id})
		if e != nil {
			return e
		}
		if n != 1 {
			return Error("REMOTE_COMMAND_CONFLICT")
		}
		return nil
	})
	close(done)
	cancel()
	var unsent interface{ NotDispatched() bool }
	if errors.As(callErr, &unsent) && unsent.NotDispatched() {
		return s.failUnsentStep(ctx, req, step, sequence, "failed", errorCode(callErr, "REMOTE_NOT_APPROVED"))
	}
	var wire connectorv1.RemoteExecutionBatchResult
	decodeErr := protojson.Unmarshal(raw, &wire)
	result := StepResult{Sequence: sequence, Status: "succeeded", ExitCode: int(wire.ExitCode), Stdout: string(wire.Stdout), Stderr: string(wire.Stderr), Truncated: wire.Truncated, Directory: wire.WorkingDirectory, SessionLost: wire.SessionLost, ErrorCode: wire.ErrorCode, OutputFiles: []File{}, ObservedAt: time.Now().UTC(), Terminated: wire.Terminated}
	state := "active"
	if callErr != nil || decodeErr != nil || wire.RequestId != req.ID.String() || wire.SessionId != step.SessionID.String() || wire.SessionGeneration != step.Generation.String() || wire.Sequence != uint32(sequence) || wire.ContextRevision != uint64(session.ContextRevision+1) {
		result.Status = "result_unknown"
		result.SessionLost = true
		result.ErrorCode = "REMOTE_RESULT_UNKNOWN"
		state = "untrusted"
	} else if wire.SessionLost || !wire.Terminated {
		result.Status = "paused_for_recovery"
		if !wire.Terminated && result.ErrorCode == "" {
			result.ErrorCode = "REMOTE_RESULT_UNKNOWN"
			result.SessionLost = true
		}
		state = "lost"
	} else if wire.ExitCode != 0 || wire.ErrorCode != "" {
		result.Status = "failed"
	}
	// Persist the command receipt and context advancement before file delivery.
	// A retry in collecting can only transfer files; it cannot invoke the batch.
	encoded, _ := json.Marshal(result)
	storedState := "collecting"
	if result.Status == "result_unknown" {
		storedState = result.Status
	}
	err = retryStateTx(ctx, s.Store, func(q *db.Queries) error {
		n, e := q.SaveRemoteStepReceipt(ctx, db.SaveRemoteStepReceiptParams{ID: step.ID, EnterpriseID: req.EnterpriseID, Status: storedState, Result: encoded})
		if e != nil {
			return e
		}
		if n != 1 {
			return Error("REMOTE_COMMAND_CONFLICT")
		}
		if e = q.TouchRemoteGeneration(ctx, db.TouchRemoteGenerationParams{SessionID: step.SessionID, Generation: step.Generation, EnterpriseID: req.EnterpriseID, Status: state}); e != nil {
			return e
		}
		if result.Status == "result_unknown" {
			_, e = q.MarkRemoteSessionLost(ctx, db.MarkRemoteSessionLostParams{ID: step.SessionID, EnterpriseID: req.EnterpriseID, Generation: step.Generation})
			return e
		}
		_, e = q.CompleteRemoteSessionBatch(ctx, db.CompleteRemoteSessionBatchParams{ID: step.SessionID, EnterpriseID: req.EnterpriseID, ActiveRequestID: uuid.NullUUID{UUID: req.ID, Valid: true}, KnownDirectory: wire.WorkingDirectory, Status: state})
		return e
	})
	if err != nil {
		return result, err
	}
	if result.Status == "result_unknown" {
		return result, nil
	}
	return s.collectKnownStep(ctx, req, step, identity, result)
}
func (s Service) collectKnownStep(ctx context.Context, req db.RemoteExecutionRequest, step Step, identity db.RemoteExecutionIdentity, result StepResult) (StepResult, error) {
	files, err := s.collectFiles(ctx, req, step, identity)
	if err != nil {
		return result, err
	}
	result.OutputFiles = files
	result.OutputsCollected = true
	encoded, _ := json.Marshal(result)
	_, err = s.Store.Queries.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: req.EnterpriseID, Status: result.Status, Result: encoded})
	return result, err
}
func (s Service) failUnsentStep(ctx context.Context, req db.RemoteExecutionRequest, step Step, sequence int, status, code string) (StepResult, error) {
	result := StepResult{Sequence: sequence, Status: status, ErrorCode: code, OutputFiles: []File{}, ObservedAt: time.Now().UTC(), Terminated: true}
	encoded, _ := json.Marshal(result)
	_, err := s.Store.Queries.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: req.EnterpriseID, Status: status, Result: encoded})
	return result, err
}
