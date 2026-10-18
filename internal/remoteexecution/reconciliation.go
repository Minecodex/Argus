package remoteexecution

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"time"
)

// Late receipts amend evidence only. They never resume the old plan or advance
// a replacement interpreter's context, even if the missing command succeeded.
func (s Service) ReconcileLateReceipts(ctx context.Context) error {
	rows, err := s.Store.Queries.ListLateRemoteReceipts(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var command connectorv1.RemoteExecutionBatch
		var wire connectorv1.RemoteExecutionBatchResult
		if protojson.Unmarshal(row.Payload, &command) != nil || protojson.Unmarshal(row.Result, &wire) != nil || wire.RequestId != row.RequestID.String() || wire.SessionId != row.SessionID.String() || wire.SessionGeneration != row.Generation.String() || wire.Sequence != uint32(row.Sequence) || wire.ContextRevision != command.ContextRevision+1 || !wire.Terminated {
			continue
		}
		req, e := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: row.RequestID, EnterpriseID: row.EnterpriseID})
		if e != nil {
			return e
		}
		var spec Spec
		if json.Unmarshal(req.Spec, &spec) != nil || Digest(spec) != req.ContentHash || row.Sequence < 1 || int(row.Sequence) > len(spec.Steps) {
			return Error("REMOTE_PLAN_INVALID")
		}
		step := spec.Steps[row.Sequence-1]
		if step.ID != row.ID || step.Generation != row.Generation || command.ApprovedContentHash != req.ContentHash || command.Script != step.Script || command.ScriptSha256 != hashBytes([]byte(step.Script)) {
			return Error("REMOTE_BINDING_INVALID")
		}
		status := "succeeded"
		if wire.ExitCode != 0 || wire.ErrorCode != "" {
			status = "failed"
		}
		if wire.SessionLost {
			status = "paused_for_recovery"
		}
		known := StepResult{Sequence: int(row.Sequence), Status: status, ExitCode: int(wire.ExitCode), Stdout: string(wire.Stdout), Stderr: string(wire.Stderr), Truncated: wire.Truncated, Directory: wire.WorkingDirectory, SessionLost: true, Terminated: true, ErrorCode: wire.ErrorCode, OutputFiles: []File{}, ObservedAt: time.Now().UTC()}
		encoded, _ := json.Marshal(known)
		payload, _ := json.Marshal(Task{Operation: "collect", RequestID: req.ID, EnterpriseID: req.EnterpriseID, StepID: step.ID})
		err = retryStateTx(ctx, s.Store, func(q *db.Queries) error {
			n, e := q.RecordLateRemoteReceipt(ctx, db.RecordLateRemoteReceiptParams{ID: row.ID, EnterpriseID: row.EnterpriseID, Result: encoded})
			if e != nil || n != 1 {
				return e
			}
			if e = q.PauseReconciledRemoteRequest(ctx, db.PauseReconciledRemoteRequestParams{ID: req.ID, EnterpriseID: req.EnterpriseID}); e != nil {
				return e
			}
			_, e = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.NewSHA1(step.ID, []byte("late-output")), EnterpriseID: uuid.NullUUID{UUID: req.EnterpriseID, Valid: true}, Queue: Queue, Payload: payload, MaxAttempts: 3, AvailableAt: timeValue(time.Now())})
			return e
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func (s Service) handleLateCollection(ctx context.Context, input Task) error {
	req, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: input.RequestID, EnterpriseID: input.EnterpriseID})
	if err != nil {
		return err
	}
	var spec Spec
	if json.Unmarshal(req.Spec, &spec) != nil || Digest(spec) != req.ContentHash {
		return Error("REMOTE_PLAN_INVALID")
	}
	rows, err := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: req.ID, EnterpriseID: req.EnterpriseID})
	if err != nil {
		return err
	}
	for n, row := range rows {
		if row.ID != input.StepID || row.Status != "collecting" {
			continue
		}
		var known StepResult
		if json.Unmarshal(row.Result, &known) != nil || !known.Terminated {
			return Error("REMOTE_PLAN_INVALID")
		}
		step := spec.Steps[n]
		identity, e := s.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: step.Identity.ID, EnterpriseID: req.EnterpriseID})
		if e != nil {
			return e
		}
		_, e = s.collectKnownStep(ctx, req, step, identity, known)
		return e
	}
	return nil
}
