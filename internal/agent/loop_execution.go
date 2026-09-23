package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/presentation"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (loop Loop) executeToolCalls(ctx context.Context, run db.Run, step db.RunStep, modelCall db.ModelCall, set *toolruntime.Set, principal toolruntime.Principal, readOnly bool, text, modelScope string, calls []*pendingToolCall) ([]toolExecutionResult, error) {
	records := make([]db.ToolCall, 0, len(calls))
	assistant := modelprovider.Message{Role: "assistant", Content: text}
	scope, err := presentation.Scope(ctx, loop.Store, run.EnterpriseID, run.ActorUserID)
	if err != nil {
		return nil, err
	}
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		for index, call := range calls {
			var input map[string]any
			if json.Unmarshal([]byte(call.Arguments), &input) != nil || input == nil {
				return toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
			}
			encoded, _ := json.Marshal(input)
			hash := sha256.Sum256(encoded)
			id := uuid.NewSHA1(modelCall.ID, []byte(call.ID))
			tool, exists := set.Lookup(call.Name)
			source := "argus"
			var connection uuid.NullUUID
			schemaHash := ""
			if exists {
				source = tool.Source
				schemaHash = tool.SchemaHash
				if parsed, e := uuid.Parse(tool.ConnectionID); e == nil {
					connection = uuid.NullUUID{UUID: parsed, Valid: true}
				}
			}
			record, err := q.CreateModelToolCall(ctx, db.CreateModelToolCallParams{ID: id, CallID: call.ID, EnterpriseID: run.EnterpriseID, RunID: run.ID, StepID: step.ID,
				ToolID: call.Name, Input: encoded, InputHash: hash[:], Source: source, ModelCallID: uuid.NullUUID{UUID: modelCall.ID, Valid: true}, CallSequence: int32(index), ConnectionID: connection, SchemaHash: schemaHash, AuthorizationScope: scope})
			if err != nil {
				return err
			}
			records = append(records, record)
			call.ID = id.String()
			assistant.ToolCalls = append(assistant.ToolCalls, modelprovider.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID,
			RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "assistant_message", ActorType: "model", ActorID: run.ModelID.String(),
			Payload: map[string]any{"content": assistant.Content, "tool_calls": assistant.ToolCalls, "authorization_scope": modelScope}, Classification: "internal"})
		return err
	})
	if err != nil {
		return nil, err
	}
	results := make([]toolExecutionResult, len(records))
	stopped := false
	for index, record := range records {
		var result toolExecutionResult
		var err error
		if stopped {
			result, err = loop.persistToolOutcome(ctx, run, record, toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_CANCELLED"})
		} else {
			result, err = loop.executePersistedTool(ctx, run, set, principal, readOnly, record)
		}
		if err != nil {
			return nil, err
		}
		results[index] = result
		stopped = stopped || result.unknown
	}
	return results, nil
}

func (loop Loop) executePersistedTool(ctx context.Context, run db.Run, set *toolruntime.Set, principal toolruntime.Principal, readOnly bool, record db.ToolCall) (toolExecutionResult, error) {
	executionCtx, cancel := loop.toolContext(ctx, run)
	defer cancel()
	current, err := loop.Store.Queries.GetRun(ctx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return toolExecutionResult{}, err
	}
	if terminalRun(current.Status) {
		return loop.persistToolOutcome(ctx, run, record, toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_CANCELLED"})
	}
	readOnly = readOnly || current.VerificationOnly
	if stored, err := loop.Store.Queries.GetPersistedToolResult(ctx, db.GetPersistedToolResultParams{ToolCallID: record.ID, EnterpriseID: run.EnterpriseID}); err == nil {
		sequence, err := loop.Store.Queries.GetToolResultEventSequence(ctx, db.GetToolResultEventSequenceParams{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, ToolCallID: record.ID.String()})
		return toolExecutionResult{message: string(stored.Projection), unknown: record.Status == "result_unknown", through: sequence}, err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return toolExecutionResult{}, err
	}
	var arguments map[string]any
	if err := json.Unmarshal(record.Input, &arguments); err != nil {
		return toolExecutionResult{}, err
	}
	tool, exists := set.Lookup(record.ToolID)
	unknown := record.Status == "dispatched" && (record.Source == "external_mcp" || record.Source == "sandbox" && (!exists || !tool.ReadOnly))
	var result toolruntime.Result
	var invokeErr error
	if unknown {
		result = toolruntime.Result{Unknown: true, Data: map[string]any{"error_code": "TOOL_RESULT_UNKNOWN", "message": "The previous invocation was dispatched but its result is unknown. It was not replayed."}}
	} else {
		beforeDispatch := func(dispatchCtx context.Context) error {
			if err := assertTaskLease(dispatchCtx); err != nil {
				return err
			}
			latest, err := loop.Store.Queries.GetRun(dispatchCtx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
			if err != nil {
				return err
			}
			if terminalRun(latest.Status) {
				return toolruntime.Error{Kind: "TOOL_CANCELLED"}
			}
			if latest.VerificationOnly && (!exists || !tool.ReadOnly) {
				return toolruntime.Error{Kind: "TOOL_READ_ONLY_REQUIRED"}
			}
			if record.Status != "prepared" {
				return nil
			}
			count, err := loop.Store.Queries.MarkToolDispatched(dispatchCtx, db.MarkToolDispatchedParams{ID: record.ID, EnterpriseID: run.EnterpriseID, ExpectedRunVersion: latest.Version})
			if err != nil {
				return err
			}
			if count != 1 {
				return toolruntime.Error{Kind: "TOOL_INVOCATION_CONFLICT"}
			}
			return nil
		}
		// Remote discovery is safe to repeat. Only the business HTTP request
		// crosses the durable dispatched boundary and becomes unreplayable.
		if record.Source != "external_mcp" {
			if err := beforeDispatch(executionCtx); err != nil {
				return loop.persistToolOutcome(ctx, run, record, toolruntime.Result{}, err)
			}
		}

		result, invokeErr = set.Invoke(executionCtx, record.ToolID, toolruntime.Invocation{Principal: principal, RunID: run.ID, StepID: record.StepID, ModelCallID: record.ModelCallID.UUID,
			ID: record.ID, CallID: record.ID.String(), Arguments: arguments, ReadOnly: readOnly, BeforeDispatch: beforeDispatch})
	}
	return loop.persistToolOutcome(ctx, run, record, result, invokeErr)
}

func (loop Loop) persistToolOutcome(ctx context.Context, run db.Run, record db.ToolCall, result toolruntime.Result, invokeErr error) (toolExecutionResult, error) {
	if err := assertTaskLease(ctx); err != nil {
		return toolExecutionResult{}, err
	}
	status, errorCode := "succeeded", ""
	if invokeErr != nil {
		status, errorCode = "failed", toolErrorCode(invokeErr)
		if errorCode == "TOOL_CANCELLED" {
			status = "cancelled"
		}
		result.Data = map[string]any{"error_code": errorCode, "message": "Tool invocation failed."}
	}
	if result.Unknown {
		status, errorCode = "result_unknown", "TOOL_RESULT_UNKNOWN"
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	full, err := json.Marshal(result.Data)
	if err != nil {
		return toolExecutionResult{}, err
	}
	if len(full) > maxToolResultBytes {
		status, errorCode = "failed", "TOOL_RESULT_TOO_LARGE"
		result.Data = map[string]any{"error_code": errorCode}
		full, _ = json.Marshal(result.Data)
	}
	resultRef := "result_" + record.ID.String()
	target := result.ToolID
	if target == "" {
		target = record.ToolID
	}
	projection, partial, err := encodeToolResultProjection(resultRef, record.ID.String(), full, result)
	if err != nil {
		return toolExecutionResult{}, err
	}
	inline := full
	objectKey := pgtype.Text{}
	if len(full) > 4<<20 {
		if loop.Objects == nil {
			return toolExecutionResult{}, toolruntime.Error{Kind: "TOOL_RESULT_STORAGE_UNAVAILABLE"}
		}
		key := fmt.Sprintf("enterprises/%s/conversations/%s/results/%s", run.EnterpriseID, run.ConversationID, record.ID)
		if err := loop.Objects.PutFile(ctx, key, bytes.NewReader(full), int64(len(full)), "application/json"); err != nil {
			return toolExecutionResult{}, err
		}
		inline = nil
		objectKey = pgtype.Text{String: key, Valid: true}
	}
	scope := record.AuthorizationScope
	fullHash, projectionHash := sha256.Sum256(full), sha256.Sum256(projection)
	var through int64
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		artifact, err := q.CreateArtifact(ctx, db.CreateArtifactParams{ID: uuid.NewSHA1(record.ID, []byte("result")), ResultRef: resultRef, EnterpriseID: run.EnterpriseID,
			ConversationID: uuid.NullUUID{UUID: run.ConversationID, Valid: true}, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, ContentType: "application/json", DataClassification: "internal", Content: inline, ObjectKey: objectKey, AuthorizationScope: scope, ContentHash: fullHash[:], ByteSize: int32(len(full))})
		if err != nil {
			return err
		}
		if _, err = q.CreateToolResult(ctx, db.CreateToolResultParams{ID: uuid.NewSHA1(record.ID, []byte("projection")), ToolCallID: record.ID, EnterpriseID: run.EnterpriseID,
			ArtifactID: artifact.ID, Projection: projection, ProjectionHash: projectionHash[:], ProjectionBytes: int32(len(projection)), Partial: partial}); err != nil {
			return err
		}
		if _, err = q.FinishToolCall(ctx, db.FinishToolCallParams{ID: record.ID, Status: status, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}}); err != nil {
			return err
		}
		event, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true},
			StepID: uuid.NullUUID{UUID: record.StepID, Valid: true}, Type: "tool_call_result", ActorType: "service", Payload: map[string]any{"tool_call_id": record.ID.String(), "tool_id": record.ToolID,
				"authorization_scope": scope, "target_tool_id": target, "source": record.Source, "status": status, "result_ref": resultRef, "projection": json.RawMessage(projection)}, ArtifactRef: resultRef, Classification: "internal"})
		if err != nil {
			return err
		}
		through = event.Sequence
		if result.Presentation != nil && status == "succeeded" {
			p := result.Presentation
			if err := q.CreateTemplateAsset(ctx, db.CreateTemplateAssetParams{Hash: p.Hash, Source: p.Template}); err != nil {
				return err
			}
			data, _ := json.Marshal(p.Data)
			refs, _ := json.Marshal(p.Resources)
			presentation, err := q.CreateToolPresentation(ctx, db.CreateToolPresentationParams{ID: uuid.NewSHA1(record.ID, []byte("presentation")), EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID,
				ToolCallID: record.ID, ToolVersion: p.Version, AuthorizationScope: scope, TemplateHash: p.Hash, DetailData: data, ResourceRefs: refs, Status: "ready"})
			if err != nil {
				return err
			}
			_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true},
				StepID: uuid.NullUUID{UUID: record.StepID, Valid: true}, Type: "tool_presentation", ActorType: "service", Payload: map[string]any{"tool_call_id": record.ID.String(), "presentation_id": presentation.ID.String(), "template_hash": p.Hash, "status": "ready"}, Classification: "internal"})
			return err
		}
		return nil
	})
	return toolExecutionResult{message: string(projection), unknown: result.Unknown, through: through}, err
}

func (loop Loop) recoverToolCalls(ctx context.Context, run db.Run, set *toolruntime.Set, p toolruntime.Principal, readOnly bool) (bool, error) {
	calls, err := loop.Store.Queries.ListUnfinishedRunToolCalls(ctx, db.ListUnfinishedRunToolCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil || len(calls) == 0 {
		return false, err
	}
	unknown := false
	for _, call := range calls {
		var result toolExecutionResult
		var err error
		if unknown {
			result, err = loop.persistToolOutcome(ctx, run, call, toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_CANCELLED"})
		} else {
			result, err = loop.executePersistedTool(ctx, run, set, p, readOnly, call)
		}
		if err != nil {
			return true, err
		}
		unknown = unknown || result.unknown
	}
	if err := fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		seen := map[uuid.UUID]bool{}
		status := "succeeded"
		if unknown {
			status = "failed"
		}
		for _, call := range calls {
			if seen[call.StepID] {
				continue
			}
			seen[call.StepID] = true
			if _, err := q.FinishRunStep(ctx, db.FinishRunStepParams{ID: call.StepID, EnterpriseID: run.EnterpriseID, Status: status}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return true, err
	}
	if unknown {
		return true, loop.finishRun(ctx, run, "failed", "result_unknown", "TOOL_RESULT_UNKNOWN")
	}

	payload, _ := json.Marshal(conversation.AgentTask{RunID: run.ID, EnterpriseID: run.EnterpriseID, Reason: "tools_recovered"})
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		_, err := q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: newAgentID(), EnterpriseID: uuid.NullUUID{UUID: run.EnterpriseID, Valid: true}, Queue: "agent",
			RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Payload: payload, MaxAttempts: 5, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
		return err
	})
	return true, err
}

func toolErrorCode(err error) string {
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "TOOL_TIMEOUT"
	case errors.Is(err, context.Canceled):
		return "TOOL_CANCELLED"
	default:
		return "TOOL_EXECUTION_FAILED"
	}
}
