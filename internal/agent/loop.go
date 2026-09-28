package agent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	modelservice "github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/objectstore"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

const (
	maxAgentTurns      = 24
	maxToolResultBytes = 64 << 20
	maxProjectionBytes = 64 << 10
)

type Loop struct {
	Objects        *objectstore.Client
	Store          *postgres.Store
	Models         modelservice.Service
	Tools          toolruntime.Factory
	EndpointPolicy modelprovider.PublicEndpointPolicy
}

type pendingToolCall struct {
	ID, Name, Arguments string
	Index               int
}

type toolExecutionResult struct {
	message string
	unknown bool
	through int64
}

func (loop Loop) Handle(ctx context.Context, task runtime.Task) error {
	var payload conversation.AgentTask
	if err := json.Unmarshal(task.Payload, &payload); err != nil || payload.RunID == uuid.Nil || payload.EnterpriseID == uuid.Nil {
		return runtime.Error{ErrorCode: "TOOL_INPUT_INVALID", Cause: errors.New("invalid agent task"), Permanent: true}
	}
	ctx, cancelLease := withTaskLease(ctx, loop.Store, task)
	defer cancelLease()
	if err := assertTaskLease(ctx); err != nil {
		return err
	}
	run, err := loop.Store.Queries.GetRun(ctx, db.GetRunParams{ID: payload.RunID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return err
	}
	if err := fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		return q.ReconcileInterruptedModelCalls(ctx, db.ReconcileInterruptedModelCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, CallKind: "inference"})
	}); err != nil {
		return err
	}

	if terminalRun(run.Status) {
		return loop.reconcileTerminalToolCalls(ctx, run)
	}
	// A worker can die after committing an unknown Tool result but before
	// committing the Run's terminal event. Recovery must not ask the model to
	// continue from that persisted result or report a successful Run.
	unknown, err := loop.Store.Queries.HasUnknownRunToolResult(ctx, db.HasUnknownRunToolResultParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return err
	}
	if unknown {
		return loop.finishRun(ctx, run, "failed", "result_unknown", "TOOL_RESULT_UNKNOWN")
	}
	user, err := loop.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: run.ActorUserID, EnterpriseID: run.EnterpriseID})
	if err != nil || user.Status != "active" || user.AuthorizationVersion != run.AuthorizationVersion {
		return loop.finishRun(ctx, run, "failed", "authorization_invalidated", "AUTHORIZATION_VERSION_STALE")
	}
	if current, paused, err := loop.reconcileActions(ctx, run); err != nil || paused {
		return err
	} else {
		run = current
	}
	revision, err := loop.Store.Queries.GetEnabledAIModelRevision(ctx, db.GetEnabledAIModelRevisionParams{ModelID: run.ModelID, EnterpriseID: run.EnterpriseID, Revision: run.ModelRevision})
	if err != nil {
		return loop.finishRun(ctx, run, "failed", "model_unavailable", "MODEL_COMPATIBILITY_FAILED")
	}
	credential, err := loop.Models.LeaseCredential(ctx, run.EnterpriseID, revision.ID)
	if err != nil {
		return err
	}
	defer clear(credential)

	provider := modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl,
		APIKey: string(credential), Client: loop.EndpointPolicy.Client()}
	messages, currentInput, modelScope, source, err := loop.messages(ctx, run)
	if err != nil {
		return err
	}
	if run.VerificationOnly {
		verification, verifyErr := loop.executionVerification(ctx, run)
		if verifyErr != nil {
			return verifyErr
		}
		messages = append(messages, modelprovider.Message{Role: "user", Content: verification})
		currentInput = verification
	}
	principal, err := (conversation.Service{Store: loop.Store}).ToolPrincipal(ctx, run.EnterpriseID, run.ActorUserID, run.ConversationID)
	if err != nil {
		return err
	}
	if loop.Tools == nil {
		return loop.finishRun(ctx, run, "failed", "tools_unavailable", "TOOL_CONFIGURATION_INVALID")
	}
	var savedTools toolruntime.Snapshot
	if json.Unmarshal(run.ToolSnapshot, &savedTools) != nil || savedTools.Hash() != run.ToolSnapshotHash {
		return loop.finishRun(ctx, run, "failed", "invalid_tool_snapshot", "TOOL_CONFIGURATION_INVALID")
	}
	toolSet, err := loop.Tools.Restore(ctx, principal, savedTools)
	if err != nil {
		var toolError toolruntime.Error
		if errors.As(err, &toolError) {
			return loop.finishRun(ctx, run, "failed", "tool_snapshot_unavailable", toolError.Kind)
		}
		return err
	}
	if recovered, err := loop.recoverToolCalls(ctx, run, toolSet, principal, run.VerificationOnly); err != nil || recovered {
		return err
	}
	usedCalls, err := loop.Store.Queries.CountRunInferenceCalls(ctx, db.CountRunInferenceCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return err
	}
	for turn := int(usedCalls); turn < maxAgentTurns; turn++ {
		if cancelled, err := loop.cancelled(ctx, run); err != nil || cancelled {
			return err
		}
		unknown, err := loop.Store.Queries.HasUnknownRunToolResult(ctx, db.HasUnknownRunToolResultParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		if unknown {
			return loop.finishRun(ctx, run, "failed", "result_unknown", "TOOL_RESULT_UNKNOWN")
		}
		if err := assertTaskLease(ctx); err != nil {
			return err
		}
		currentScope, scopeErr := presentation.Scope(ctx, loop.Store, run.EnterpriseID, run.ActorUserID)
		if scopeErr != nil || currentScope != modelScope {
			return loop.finishRun(ctx, run, "failed", "authorization_invalidated", "AUTHORIZATION_VERSION_STALE")
		}
		turnTools := toolSet.Models()
		projection, err := loop.context(ctx, run, revision, messages, currentInput, turnTools, source)
		if err != nil {
			return loop.waitForCompaction(ctx, run, "CONTEXT_TOO_LARGE")
		}
		request := modelprovider.Request{Model: revision.ProviderModelID, Messages: projection.Messages, Tools: turnTools, MaxTokens: int(revision.MaxOutputTokens)}
		wireBytes, err := provider.RequestBytes(request)
		if err != nil {
			return err
		}
		// For arbitrary OpenAI-compatible tokenizers the wire byte count is a
		// conservative upper bound, shared with message preflight. Never rely
		// on English characters-per-token estimates for Unicode or schemas.
		projection.EstimatedTokens = wireBytes
		if projection.NeedsHardCompaction() {
			return loop.waitForCompaction(ctx, run, "")
		}
		if projection.NeedsSoftCompaction() {
			_ = loop.enqueueCompaction(ctx, run, "soft_limit")
		}

		step, updatedRun, err := loop.startStep(ctx, run, projection)
		if err != nil {
			return err
		}
		run = updatedRun
		modelCall, err := loop.createModelCall(ctx, run, step, revision, projection, source)
		if err != nil {
			_ = loop.failStep(ctx, run, step)
			return err
		}
		reservedAmount := tokenAmount(int64(projection.EstimatedTokens), int64(revision.MaxOutputTokens), revision.InputPricePerMillion, revision.OutputPricePerMillion)
		reservation, err := loop.Models.ReserveQuota(ctx, modelCall, user.DepartmentID, user.ID, reservedAmount)
		if err != nil {
			_ = loop.finishModelCall(ctx, modelCall, revision, modelprovider.TokenUsage{}, 0, "quota_exceeded", "failed", "MODEL_QUOTA_EXCEEDED", tokenAmount(0, 0, revision.InputPricePerMillion, revision.OutputPricePerMillion))
			_ = loop.failStep(ctx, run, step)
			return loop.finishRun(ctx, run, "failed", "quota_exceeded", "MODEL_QUOTA_EXCEEDED")
		}
		actualHash, err := provider.RequestHash(request)
		if err != nil {
			return err
		}
		hashBytes, err := hex.DecodeString(actualHash)
		if err != nil {
			return err
		}
		if err := persistModelDispatch(ctx, loop.Store, db.MarkModelDispatchedParams{ID: modelCall.ID, EnterpriseID: run.EnterpriseID, ProjectionHash: hashBytes, ToolSnapshotHash: run.ToolSnapshotHash, CapabilitySnapshot: revision.Capabilities}); err != nil {
			return err
		}
		started := time.Now()
		var text strings.Builder
		var delta strings.Builder
		lastDeltaFlush := time.Now()
		flushDelta := func() error {
			if delta.Len() == 0 {
				return nil
			}
			value := delta.String()
			delta.Reset()
			lastDeltaFlush = time.Now()
			return loop.persistDelta(ctx, run, step, value)
		}
		calls := map[string]*pendingToolCall{}
		var usage modelprovider.TokenUsage
		stopReason := ""
		requestContext, stopRequest := loop.toolContext(ctx, run)
		err = provider.Stream(requestContext, request, func(event modelprovider.Event) error {
			usage.Observe(event)
			if cancelled, checkErr := loop.cancelled(ctx, run); checkErr != nil || cancelled {
				if checkErr != nil {
					return checkErr
				}
				return context.Canceled
			}
			switch event.Type {
			case "text_delta":
				text.WriteString(event.Text)
				delta.WriteString(event.Text)
				if delta.Len() >= 8<<10 || time.Since(lastDeltaFlush) >= 250*time.Millisecond {
					return flushDelta()
				}
			case "tool_call_done":
				key := event.ToolCallID
				if key == "" {
					key = fmt.Sprintf("call_%d", len(calls)+1)
				}
				call := calls[key]
				if call == nil {
					call = &pendingToolCall{ID: key, Index: event.Index}
					calls[key] = call
				}
				if event.ToolName != "" {
					call.Name = event.ToolName
				}
				if event.Type == "tool_call_done" {
					call.Arguments = event.Arguments
				} else {
					call.Arguments += event.Arguments
				}
			case "usage":
			case "completed":
				if event.StopReason != "" {
					stopReason = event.StopReason
				}
			}
			return nil
		})
		stopRequest()
		if err == nil {
			if flushErr := flushDelta(); flushErr != nil {
				err = flushErr
			}
		}
		if err != nil {
			code := "MODEL_COMPATIBILITY_FAILED"
			if errors.Is(err, modelprovider.ErrInvalidUsage) {
				usage.Invalidate()
				code = "MODEL_USAGE_INVALID"
			}
			if finishErr := loop.finishModelCall(ctx, modelCall, revision, usage, time.Since(started), stopReason, "failed", code, reservation.ReservedAmount); finishErr != nil {
				return finishErr
			}
			if settleErr := loop.Models.SettleQuota(ctx, reservation, usageAmount(usage, revision, reservation.ReservedAmount)); settleErr != nil {
				return settleErr
			}
			if stepErr := loop.failStep(ctx, run, step); stepErr != nil {
				return stepErr
			}
			if code == "MODEL_USAGE_INVALID" {
				return loop.finishRun(ctx, run, "failed", "model_usage_invalid", code)
			}
			if errors.Is(err, context.Canceled) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return loop.finishRun(ctx, run, "cancelled", "request_cancelled", "")
			}
			return runtime.Error{ErrorCode: "MODEL_COMPATIBILITY_FAILED", Cause: err}
		}
		usage.Estimate(int64(projection.EstimatedTokens), int64((text.Len()+3)/4))
		if err := loop.finishModelCall(ctx, modelCall, revision, usage, time.Since(started), stopReason, "succeeded", "", reservation.ReservedAmount); err != nil {
			_ = loop.failStep(ctx, run, step)
			return err
		}
		if err := loop.Models.SettleQuota(ctx, reservation, tokenAmount(usage.Input, usage.Output, revision.InputPricePerMillion, revision.OutputPricePerMillion)); err != nil {
			_ = loop.failStep(ctx, run, step)
			return err
		}
		if len(calls) == 0 {
			if err := loop.persistAssistant(ctx, run, step, text.String(), modelScope, usage); err != nil {
				return err
			}
			return loop.finishRun(ctx, run, "succeeded", stopReason, "")
		}
		if !allowsToolExecution(stopReason) {
			if finishErr := persistStepStatus(ctx, loop.Store, db.FinishRunStepParams{ID: step.ID, EnterpriseID: run.EnterpriseID, Status: "failed"}); finishErr != nil {
				return finishErr
			}
			return loop.finishRun(ctx, run, "failed", "model_output_incomplete", "TOOL_INPUT_INVALID")
		}
		ordered := orderedCalls(calls)
		toolResults, err := loop.executeToolCalls(ctx, run, step, modelCall, toolSet, principal, run.VerificationOnly, text.String(), modelScope, ordered)
		if err != nil {
			_ = loop.failStep(ctx, run, step)
			return err
		}
		assistant := modelprovider.Message{Role: "assistant", Content: text.String()}
		for _, call := range ordered {
			assistant.ToolCalls = append(assistant.ToolCalls, modelprovider.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		messages = append(messages, assistant)
		for index, call := range ordered {
			messages = append(messages, modelprovider.Message{Role: "tool", ToolCallID: call.ID, Content: toolResults[index].message})
			source.ThroughSequence = max(source.ThroughSequence, toolResults[index].through)
			if toolResults[index].unknown {
				return loop.finishRun(ctx, run, "failed", "result_unknown", "TOOL_RESULT_UNKNOWN")
			}
		}
		if err := persistStepStatus(ctx, loop.Store, db.FinishRunStepParams{ID: step.ID, EnterpriseID: run.EnterpriseID, Status: "succeeded"}); err != nil {
			return err
		}
		if current, paused, err := loop.reconcileActions(ctx, run); err != nil || paused {
			return err
		} else {
			run = current
		}

	}
	return loop.finishRun(ctx, run, "failed", "output_limit", "RUN_STEP_LIMIT_REACHED")
}

func (loop Loop) context(ctx context.Context, run db.Run, revision db.AiModelRevision, messages []modelprovider.Message, current string, tools []modelprovider.Tool, source ContextSource) (ContextProjection, error) {
	checkpoint, _, err := conversation.ContextFacts(ctx, loop.Store.Queries, run.EnterpriseID, run.ActorUserID, run.ConversationID)
	if err != nil {
		return ContextProjection{}, err
	}
	p, err := (conversation.Service{Store: loop.Store}).ToolPrincipal(ctx, run.EnterpriseID, run.ActorUserID, run.ConversationID)
	if err != nil {
		return ContextProjection{}, err
	}
	selection, err := dashboardcontext.ForRun(ctx, loop.Store.Queries, p, run.ID)
	if err != nil {
		return ContextProjection{}, err
	}
	checkpoint["dashboard"], err = dashboardcontext.Facts(ctx, loop.Store.Queries, p, selection, run.ID)
	if err != nil {
		return ContextProjection{}, err
	}
	var snapshot any
	if source.Snapshot != nil {
		active := source.Snapshot
		snapshot = map[string]any{"id": active.ID, "typed_checkpoint": json.RawMessage(active.TypedCheckpoint), "narrative_summary": active.NarrativeSummary, "source_hash": hex.EncodeToString(active.SourceHash)}
	}
	return AssembleContext(ContextInput{ContextWindow: int(revision.ContextWindowTokens), MaxOutput: int(revision.MaxOutputTokens), System: messages[0], ToolCatalog: tools, Checkpoint: checkpoint, Snapshot: snapshot, RecentTail: messages[1:], CurrentInput: current})
}

func (loop Loop) startStep(ctx context.Context, run db.Run, projection ContextProjection) (step db.RunStep, updated db.Run, err error) {
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		sequence, err := q.NextRunStepSequence(ctx, db.NextRunStepSequenceParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		step, err = q.CreateRunStep(ctx, db.CreateRunStepParams{ID: newAgentID(), RunID: run.ID, EnterpriseID: run.EnterpriseID, Sequence: sequence, StepType: "model_call", Status: "running"})
		if err != nil {
			return err
		}
		updated, err = q.SetRunCurrentStep(ctx, db.SetRunCurrentStepParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Status: "running", CurrentStepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Version: run.Version})
		return err
	})
	return
}

func (loop Loop) createModelCall(ctx context.Context, run db.Run, step db.RunStep, revision db.AiModelRevision, projection ContextProjection, source ContextSource) (call db.ModelCall, err error) {
	hash, _ := hex.DecodeString(projection.Hash)
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		var createErr error
		call, createErr = q.CreateModelCall(ctx, db.CreateModelCallParams{ID: newAgentID(), EnterpriseID: run.EnterpriseID, RunID: run.ID, StepID: step.ID, ModelID: run.ModelID, ModelRevision: run.ModelRevision, CallKind: "inference", ContextSnapshotID: source.snapshotID(), ContextSnapshotHash: source.snapshotHash(), ContextFromSequence: source.FromSequence, ContextThroughSequence: source.ThroughSequence, ProjectionHash: hash, InputPriceSnapshot: revision.InputPricePerMillion, OutputPriceSnapshot: revision.OutputPricePerMillion})
		return createErr
	})
	return
}
func (loop Loop) finishModelCall(ctx context.Context, call db.ModelCall, revision db.AiModelRevision, usage modelprovider.TokenUsage, latency time.Duration, reason, status, errorCode string, reserved pgtype.Numeric) error {
	amount := usageAmount(usage, revision, reserved)
	return fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		_, err := q.FinishModelCall(ctx, db.FinishModelCallParams{ID: call.ID, EnterpriseID: call.EnterpriseID, InputTokens: usage.Input, OutputTokens: usage.Output, CachedInputTokens: usage.CachedInput, CachedInputUsageSource: usage.CachedSource(), InputUsageSource: usage.InputSource(), OutputUsageSource: usage.OutputSource(), Amount: amount, LatencyMs: latency.Milliseconds(), StopReason: pgtype.Text{String: reason, Valid: reason != ""}, Status: status, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}})
		return err
	})
}

func (loop Loop) failStep(ctx context.Context, run db.Run, step db.RunStep) error {
	err := persistStepStatus(ctx, loop.Store, db.FinishRunStepParams{ID: step.ID, EnterpriseID: run.EnterpriseID, Status: "failed"})
	return err
}

func (loop Loop) persistAssistant(ctx context.Context, run db.Run, step db.RunStep, text, scope string, usage modelprovider.TokenUsage) error {
	return fencedEventTransaction(ctx, loop.Store, func(q *db.Queries) error {
		if _, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "assistant_message", ActorType: "model", ActorID: run.ModelID.String(), Payload: map[string]any{"content": text, "authorization_scope": scope}, Classification: "internal"}); err != nil {
			return err
		}
		_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "model_usage", ActorType: "system", Payload: map[string]any{"input_tokens": usage.Input, "output_tokens": usage.Output, "input_usage_source": usage.InputSource(), "output_usage_source": usage.OutputSource(), "usage_complete": usage.Complete(), "cached_input_tokens": usage.CachedInput, "cached_input_usage_source": usage.CachedSource(), "cached_usage_complete": usage.CacheComplete()}, Classification: "internal"})
		if err != nil {
			return err
		}
		_, err = q.FinishRunStep(ctx, db.FinishRunStepParams{ID: step.ID, EnterpriseID: run.EnterpriseID, Status: "succeeded"})
		return err
	})
}
func (loop Loop) persistDelta(ctx context.Context, run db.Run, step db.RunStep, text string) error {
	return fencedEventTransaction(ctx, loop.Store, func(q *db.Queries) error {
		_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, StepID: uuid.NullUUID{UUID: step.ID, Valid: true}, Type: "run_state_changed", ActorType: "model", Payload: map[string]any{"agent_event_type": "message_delta", "delta": text}, Classification: "internal"})
		return err
	})
}
func (loop Loop) finishRun(ctx context.Context, run db.Run, status, reason, errorCode string) error {
	return fencedRunTransition(ctx, loop.Store, func(q *db.Queries) error {
		current, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		_, err = conversation.FinishRunRecord(ctx, q, current, status, reason, errorCode, "system", "")
		return err
	})
}
func (loop Loop) cancelled(ctx context.Context, run db.Run) (bool, error) {
	value, err := loop.Store.Queries.GetRun(ctx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
	return err == nil && value.Status == "cancelled", err
}
func compactionTask(run db.Run, reason string) db.CreateRuntimeTaskParams {
	payload, _ := json.Marshal(conversation.AgentTask{RunID: run.ID, EnterpriseID: run.EnterpriseID, Reason: reason})
	return db.CreateRuntimeTaskParams{ID: newAgentID(), EnterpriseID: uuid.NullUUID{UUID: run.EnterpriseID, Valid: true}, Queue: "compaction", RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Payload: payload, MaxAttempts: 3, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}
}
func (loop Loop) waitForCompaction(ctx context.Context, run db.Run, errorCode string) error {
	return fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		current, err := q.GetRun(ctx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		if terminalRun(current.Status) {
			return nil
		}
		if _, err := q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Status: "waiting_system", StopReason: pgtype.Text{String: "context_compaction", Valid: true}, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}, Version: current.Version}); err != nil {
			return err
		}
		_, err = q.CreateRuntimeTask(ctx, compactionTask(run, "hard_limit"))
		return err
	})
}
func (loop Loop) enqueueCompaction(ctx context.Context, run db.Run, reason string) error {
	return fencedTransaction(ctx, loop.Store, func(q *db.Queries) error { _, err := q.CreateRuntimeTask(ctx, compactionTask(run, reason)); return err })
}
func orderedCalls(values map[string]*pendingToolCall) []*pendingToolCall {
	result := make([]*pendingToolCall, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result
}

func terminalRun(status string) bool {
	return conversation.TerminalRun(status)
}

func allowsToolExecution(stopReason string) bool {
	switch strings.ToLower(strings.TrimSpace(stopReason)) {
	case "stop", "tool_calls", "completed":
		return true
	default:
		return false
	}
}
func opaqueRef(prefix string) (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}
func tokenAmount(input, output int64, inputPrice, outputPrice pgtype.Numeric) pgtype.Numeric {
	in, _ := inputPrice.Float64Value()
	out, _ := outputPrice.Float64Value()
	amount := float64(input)*in.Float64/1_000_000 + float64(output)*out.Float64/1_000_000
	var result pgtype.Numeric
	_ = result.Scan(fmt.Sprintf("%.8f", amount))
	return result
}
func newAgentID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}
