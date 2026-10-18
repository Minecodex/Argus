package remoteexecution

import (
	"context"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A runtime retry budget is not permission to repeat an unknown remote effect.
// Converge the request so the user can review a new recovery revision.
func (s Service) HandleExhausted(ctx context.Context, task runtime.Task, _ error) error {
	var input Task
	if json.Unmarshal(task.Payload, &input) != nil || input.Operation == "reclaim" {
		return nil
	}
	request, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: input.RequestID, EnterpriseID: input.EnterpriseID})
	if err != nil {
		return err
	}
	if request.Status != "ready" && request.Status != "running" && request.Status != "stopping" {
		if input.Operation != "collect" {
			return nil
		}
	}
	steps, err := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: request.ID, EnterpriseID: request.EnterpriseID})
	if err != nil {
		return err
	}
	state := "failed"
	code := "REMOTE_WORKER_INTERRUPTED"
	for _, step := range steps {
		if step.Status == "succeeded" && state == "failed" {
			state = "partial_failed"
		}
		if step.Status == "collecting" {
			var known StepResult
			if json.Unmarshal(step.Result, &known) != nil {
				return Error("REMOTE_PLAN_INVALID")
			}
			known.OutputsCollected = false
			encoded, _ := json.Marshal(known)
			if _, err = s.Store.Queries.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: request.EnterpriseID, Status: known.Status, Result: encoded}); err != nil {
				return err
			}
			if state != "result_unknown" {
				state = "partial_failed"
				code = "REMOTE_OUTPUT_COLLECTION_FAILED"
			}
		}
		if step.Status == "running" || step.Status == "result_unknown" {
			state = "result_unknown"
			code = "REMOTE_WORKER_INTERRUPTED"
			unknown := StepResult{Sequence: int(step.Sequence), Status: "result_unknown", SessionLost: true, ErrorCode: code, OutputFiles: []File{}}
			encoded, _ := json.Marshal(unknown)
			if _, err = s.Store.Queries.FinishRemoteStep(ctx, db.FinishRemoteStepParams{ID: step.ID, EnterpriseID: request.EnterpriseID, Status: "result_unknown", Result: encoded}); err != nil {
				return err
			}
			session, e := s.Store.Queries.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: request.EnterpriseID})
			if e != nil {
				return e
			}
			if _, e = s.Store.Queries.MarkRemoteSessionLost(ctx, db.MarkRemoteSessionLostParams{ID: session.ID, EnterpriseID: request.EnterpriseID, Generation: session.Generation}); e != nil {
				return e
			}
		}
	}
	if input.Operation == "collect" {
		return nil
	}
	return s.finish(ctx, request, state, code)
}
