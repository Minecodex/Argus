package remoteexecution

import (
	"context"
	"encoding/json"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"time"
)

// A user may still request termination after a missing receipt. This task can
// send only session-stop controls and never re-enters the command dispatcher.
func (s Service) handleStop(ctx context.Context, input Task) error {
	req, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: input.RequestID, EnterpriseID: input.EnterpriseID})
	if err != nil {
		return err
	}
	if !req.StopRequested || req.Status != "stopping" {
		return nil
	}
	var spec Spec
	if json.Unmarshal(req.Spec, &spec) != nil || Digest(spec) != req.ContentHash {
		return Error("REMOTE_PLAN_INVALID")
	}
	records, err := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: req.ID, EnterpriseID: req.EnterpriseID})
	if err != nil {
		return err
	}
	confirmed := true
	for n, record := range records {
		var fact StepResult
		_ = json.Unmarshal(record.Result, &fact)
		if record.CommandID == "" || fact.Terminated {
			continue
		}
		step := spec.Steps[n]
		identity, e := s.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: step.Identity.ID, EnterpriseID: req.EnterpriseID})
		if e != nil {
			confirmed = false
			continue
		}
		if s.Transport == nil {
			confirmed = false
			continue
		}
		raw, e := s.Transport.Call(ctx, identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "stop", SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: req.ID.String()}, req.ID.String()+"/stop/"+step.ID.String()+"/explicit/"+req.UpdatedAt.Time.Format(time.RFC3339Nano), 15*time.Second, nil)
		var receipt connectorv1.RemoteExecutionControlResult
		if e != nil || protojson.Unmarshal(raw, &receipt) != nil || receipt.Status != "stopped" || receipt.SessionGeneration != step.Generation.String() {
			confirmed = false
			continue
		}
		fact.Terminated = true
		encoded, _ := json.Marshal(fact)
		if _, err = s.Store.Queries.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: req.EnterpriseID, Status: record.Status, Result: encoded}); err != nil {
			return err
		}
	}
	status, requestStatus, code := "unknown", "result_unknown", "REMOTE_STOP_UNKNOWN"
	if confirmed {
		status = "confirmed"
		requestStatus = "cancelled"
		code = "REMOTE_STOPPED"
	}
	if _, err = s.Store.Queries.RecordRemoteStopReceipt(ctx, db.RecordRemoteStopReceiptParams{ID: req.ID, EnterpriseID: req.EnterpriseID, StopStatus: status}); err != nil {
		return err
	}
	return s.finish(ctx, req, requestStatus, code)
}
