package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	modelservice "github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Compactor struct {
	Store          *postgres.Store
	Models         modelservice.Service
	EndpointPolicy modelprovider.PublicEndpointPolicy
}

func (compactor Compactor) Handle(ctx context.Context, task runtime.Task) error {
	var payload conversation.AgentTask
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return runtime.Error{ErrorCode: "TOOL_INPUT_INVALID", Cause: err, Permanent: true}
	}
	ctx, cancelLease := withTaskLease(ctx, compactor.Store, task)
	defer cancelLease()
	if err := assertTaskLease(ctx); err != nil {
		return err
	}
	run, err := compactor.Store.Queries.GetRun(ctx, db.GetRunParams{ID: payload.RunID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return err
	}
	// A finished Run can still sponsor conversation-wide manual/background
	// compaction. Only its obsolete hard-limit recovery task should be skipped.
	if terminalRun(run.Status) && requiresCompactionResume(payload.Reason) {
		return nil
	}
	if err := fencedTransaction(ctx, compactor.Store, func(q *db.Queries) error {
		return q.ReconcileInterruptedModelCalls(ctx, db.ReconcileInterruptedModelCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, CallKind: "compaction"})
	}); err != nil {
		return err
	}

	conversationRow, err := compactor.Store.Queries.GetConversation(ctx, db.GetConversationParams{ID: run.ConversationID, EnterpriseID: run.EnterpriseID, OwnerUserID: run.ActorUserID})
	if err != nil {
		return err
	}
	workspace, err := conversation.CurrentWorkspaceContext(ctx, compactor.Store.Queries, run.EnterpriseID, run.ConversationID)
	if err != nil {
		return err
	}
	scope, err := presentation.Scope(ctx, compactor.Store, run.EnterpriseID, run.ActorUserID)
	if err != nil {
		return err
	}
	previous, previousErr := compactor.Store.Queries.GetActiveConversationSnapshot(ctx, db.GetActiveConversationSnapshotParams{ConversationID: run.ConversationID, EnterpriseID: run.EnterpriseID})
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		return previousErr
	}
	after := int64(0)
	previousSummary := ""
	origin := ContextSource{FromSequence: 1}
	if previousErr == nil && snapshotInScope(previous, scope) {
		after = previous.SourceThroughSequence
		previousSummary = previous.NarrativeSummary
		origin.Snapshot = &previous
	}
	events := []db.ConversationEvent{}
	for {
		page, err := compactor.Store.Queries.ListConversationContextEvents(ctx, db.ListConversationContextEventsParams{ConversationID: run.ConversationID, EnterpriseID: run.EnterpriseID, Sequence: after, Limit: 500})
		if err != nil {
			return err
		}
		events = append(events, page...)
		if len(page) < 500 {
			break
		}
		after = page[len(page)-1].Sequence
	}
	if requiresCompactionResume(payload.Reason) {
		if fits, err := compactor.currentContextFits(ctx, run); err != nil {
			return err
		} else if fits {
			return fencedTransaction(ctx, compactor.Store, func(q *db.Queries) error { return resumeCompactedRun(ctx, q, run) })
		}
	}
	through, firstKept, ok := ChooseCompactionBoundary(events)
	if !ok {
		if requiresCompactionResume(payload.Reason) {
			return compactor.fail(ctx, run, "CONTEXT_COMPACTION_FAILED")
		}
		return nil
	}
	selected := make([]db.ConversationEvent, 0)
	for _, event := range events {
		if event.Sequence <= through {
			selected = append(selected, event)
		}
	}
	source, err := json.Marshal(map[string]any{"authorization_scope": scope, "workspace": workspace, "previous_summary": previousSummary, "events": selected})
	if err != nil {
		return err
	}
	sourceHash := sha256.Sum256(source)
	if _, err := compactor.Store.Queries.GetContextSnapshotBySourceHash(ctx, db.GetContextSnapshotBySourceHashParams{
		ConversationID: run.ConversationID, EnterpriseID: run.EnterpriseID, SourceHash: sourceHash[:],
	}); err == nil {
		if requiresCompactionResume(payload.Reason) {
			return compactor.fail(ctx, run, "CONTEXT_COMPACTION_FAILED")
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	summary, summaryTokens, err := compactor.generateSummary(ctx, run, selected, scope, workspace, origin)
	if err != nil {
		code := "CONTEXT_COMPACTION_FAILED"
		invalid := errors.Is(err, modelprovider.ErrInvalidUsage)
		if invalid {
			code = "MODEL_USAGE_INVALID"
		}
		if requiresCompactionResume(payload.Reason) {
			return compactor.fail(ctx, run, code)
		}
		return runtime.Error{ErrorCode: code, Cause: err, Permanent: invalid}
	}
	typedCheckpoint, err := json.Marshal(scopedCheckpoint{AuthorizationScope: scope, Execution: json.RawMessage(run.Checkpoint), Workspace: &workspace})
	if err != nil {
		return err
	}
	snapshotPayload, _ := json.Marshal(map[string]any{"typed_checkpoint": json.RawMessage(typedCheckpoint), "narrative_summary": summary, "source_hash": sourceHash[:]})
	snapshotHash := sha256.Sum256(snapshotPayload)
	err = fencedTransaction(ctx, compactor.Store, func(q *db.Queries) error {
		revision, claimErr := q.ClaimContextRevision(ctx, db.ClaimContextRevisionParams{ID: run.ConversationID, EnterpriseID: run.EnterpriseID, ContextRevision: conversationRow.ContextRevision})
		if errors.Is(claimErr, pgx.ErrNoRows) {
			// A concurrent compactor won. Retry from its committed snapshot;
			// acknowledging this task here can strand a hard-limit Run.
			return runtime.Error{ErrorCode: "CONTEXT_REVISION_CONFLICT", Cause: claimErr}
		}
		if claimErr != nil {
			return claimErr
		}
		if _, existingErr := q.GetContextSnapshotBySourceHash(ctx, db.GetContextSnapshotBySourceHashParams{
			ConversationID: run.ConversationID, EnterpriseID: run.EnterpriseID, SourceHash: sourceHash[:],
		}); existingErr == nil {
			return nil
		} else if !errors.Is(existingErr, pgx.ErrNoRows) {
			return existingErr
		}
		if err := q.SupersedeContextSnapshots(ctx, db.SupersedeContextSnapshotsParams{ConversationID: run.ConversationID, EnterpriseID: run.EnterpriseID}); err != nil {
			return err
		}
		snapshot, createErr := q.CreateContextSnapshot(ctx, db.CreateContextSnapshotParams{ID: newAgentID(), EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: run.ID, Revision: int32(revision),
			SourceFromSequence: events[0].Sequence, SourceThroughSequence: through, FirstKeptSequence: firstKept, TypedCheckpoint: typedCheckpoint, NarrativeSummary: summary,
			CompactionModelID: run.ModelID, CompactionModelRevision: run.ModelRevision, PromptVersion: "argus.compaction/v2", EstimatedTokensBefore: int32((len(source) + 3) / 4),
			ActualTokensAfter: int32(summaryTokens), SourceHash: sourceHash[:], SnapshotHash: snapshotHash[:], Status: "active"})
		if errors.Is(createErr, pgx.ErrNoRows) {
			return nil
		}
		if createErr != nil {
			return createErr
		}
		if _, eventErr := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "context_compacted", ActorType: "system", Payload: map[string]any{"snapshot_id": snapshot.ID.String(), "source_through_sequence": through, "first_kept_sequence": firstKept, "estimated_tokens_before": (len(source) + 3) / 4, "estimated_tokens_after": (len(snapshotPayload) + 3) / 4}, Classification: "internal"}); eventErr != nil {
			return eventErr
		}
		if !requiresCompactionResume(payload.Reason) {
			return nil
		}
		return resumeCompactedRun(ctx, q, run)
	})
	return err
}

func requiresCompactionResume(reason string) bool {
	return reason == "hard_limit"
}

func (compactor Compactor) generateSummary(ctx context.Context, run db.Run, events []db.ConversationEvent, scope string, workspace conversation.WorkspaceContext, origin ContextSource) (string, int, error) {
	revision, err := compactor.Store.Queries.GetEnabledAIModelRevision(ctx, db.GetEnabledAIModelRevisionParams{ModelID: run.ModelID, EnterpriseID: run.EnterpriseID, Revision: run.ModelRevision})
	if err != nil {
		return "", 0, err
	}
	credential, err := compactor.Models.LeaseCredential(ctx, run.EnterpriseID, revision.ID)
	if err != nil {
		return "", 0, err
	}
	defer clear(credential)
	previousSummary := ""
	if origin.Snapshot != nil {
		previousSummary = origin.Snapshot.NarrativeSummary
	}
	visible := []map[string]any{}
	for _, event := range events {
		if event.EventType != "user_message" && event.EventType != "assistant_message" && event.EventType != "tool_call_result" && event.EventType != "execution_update" && event.EventType != "pending_action_created" {
			continue
		}
		data, err := contextEventPayload(event.EventType, event.Payload, scope)
		if err != nil {
			return "", 0, err
		}
		if event.EventType == "user_message" {
			var payload struct {
				Files []map[string]any `json:"files"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return "", 0, err
			}
			if len(payload.Files) > 0 {
				files, err := conversation.ProjectHistoricalFiles(ctx, compactor.Store.Queries, run.EnterpriseID, run.ConversationID, workspace, payload.Files)
				if err != nil {
					return "", 0, err
				}
				data["files"] = files
			}
		}
		visible = append(visible, map[string]any{"event_type": event.EventType, "sequence": event.Sequence, "payload": sanitize(data)})
	}
	safeEvents, err := json.Marshal(map[string]any{"workspace": workspace, "previous_summary": previousSummary, "new_events": visible})
	if err != nil {
		return "", 0, err
	}
	projectionHash := sha256.Sum256(safeEvents)
	var step db.RunStep
	var call db.ModelCall
	err = fencedTransaction(ctx, compactor.Store, func(q *db.Queries) error {
		sequence, err := q.NextRunStepSequence(ctx, db.NextRunStepSequenceParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		step, err = q.CreateRunStep(ctx, db.CreateRunStepParams{ID: newAgentID(), RunID: run.ID, EnterpriseID: run.EnterpriseID, Sequence: sequence, StepType: "context_compaction", Status: "running"})
		if err != nil {
			return err
		}
		call, err = q.CreateModelCall(ctx, db.CreateModelCallParams{ID: newAgentID(), EnterpriseID: run.EnterpriseID, RunID: run.ID, StepID: step.ID, ModelID: run.ModelID, ModelRevision: run.ModelRevision, CallKind: "compaction", ContextSnapshotID: origin.snapshotID(), ContextSnapshotHash: origin.snapshotHash(), ContextFromSequence: origin.FromSequence, ContextThroughSequence: events[len(events)-1].Sequence, ProjectionHash: projectionHash[:], InputPriceSnapshot: revision.InputPricePerMillion, OutputPriceSnapshot: revision.OutputPricePerMillion})
		return err
	})
	if err != nil {
		return "", 0, err
	}
	user, err := compactor.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: run.ActorUserID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return "", 0, err
	}
	inputEstimate := int64((len(safeEvents) + 3) / 4)
	reservation, err := compactor.Models.ReserveQuota(ctx, call, user.DepartmentID, user.ID,
		tokenAmount(inputEstimate, min(int64(revision.MaxOutputTokens), 2048), revision.InputPricePerMillion, revision.OutputPricePerMillion))
	if err != nil {
		return "", 0, err
	}
	provider := modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl,
		APIKey: string(credential), Client: compactor.EndpointPolicy.Client()}
	request := modelprovider.Request{Model: revision.ProviderModelID, MaxTokens: min(int(revision.MaxOutputTokens), 2048), Messages: []modelprovider.Message{
		{Role: "system", Content: "Summarize the conversation facts for future context. Do not include credentials, private plans, tokens, prompts, or hidden tool data. Current workspace facts override historical availability claims. Preserve deleted/unavailable file reference status; never assert that files from an earlier workspace still exist."},
		{Role: "user", Content: string(safeEvents)},
	}}
	actualHash, err := provider.RequestHash(request)
	if err != nil {
		return "", 0, err
	}
	hashBytes, err := hex.DecodeString(actualHash)
	if err != nil {
		return "", 0, err
	}
	currentScope, err := presentation.Scope(ctx, compactor.Store, run.EnterpriseID, run.ActorUserID)
	if err != nil || currentScope != scope {
		return "", 0, fmt.Errorf("compaction authorization changed before dispatch")
	}
	if err := persistModelDispatch(ctx, compactor.Store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: run.EnterpriseID, ProjectionHash: hashBytes, CapabilitySnapshot: revision.Capabilities}); err != nil {
		return "", 0, err
	}
	var output strings.Builder
	var usage modelprovider.TokenUsage
	completed := false
	started := time.Now()
	err = provider.Stream(ctx, request, func(event modelprovider.Event) error {
		if event.Type == "completed" {
			completed = event.StopReason == "stop" || event.StopReason == "completed"
		}
		if event.Type == "tool_call_done" {
			return fmt.Errorf("compaction returned a tool call")
		}

		if event.Type == "text_delta" {
			output.WriteString(event.Text)
		}
		usage.Observe(event)
		return nil
	})
	if errors.Is(err, modelprovider.ErrInvalidUsage) {
		usage.Invalidate()
	}
	usage.Estimate(inputEstimate, int64((output.Len()+3)/4))
	amount := usageAmount(usage, revision, reservation.ReservedAmount)
	status, code := "succeeded", ""
	if err == nil && !completed {
		err = fmt.Errorf("compaction response is incomplete")
	}
	if err != nil || strings.TrimSpace(output.String()) == "" {
		status, code = "failed", "CONTEXT_COMPACTION_FAILED"
		if usage.Invalid() {
			code = "MODEL_USAGE_INVALID"
		}
	}
	finishErr := fencedTransaction(ctx, compactor.Store, func(q *db.Queries) error {
		_, err := q.FinishModelCall(ctx, db.FinishModelCallParams{ID: call.ID, EnterpriseID: call.EnterpriseID,
			InputTokens: usage.Input, OutputTokens: usage.Output, CachedInputTokens: usage.CachedInput, CachedInputUsageSource: usage.CachedSource(), InputUsageSource: usage.InputSource(), OutputUsageSource: usage.OutputSource(), Amount: amount, LatencyMs: time.Since(started).Milliseconds(),
			StopReason: pgtype.Text{String: "compaction", Valid: true}, Status: status, ErrorCode: pgtype.Text{String: code, Valid: code != ""}})
		return err
	})
	if finishErr == nil {
		finishErr = compactor.Models.SettleQuota(ctx, reservation, amount)
	}
	if finishErr != nil {
		return "", 0, finishErr
	}
	if stepErr := persistStepStatus(ctx, compactor.Store, db.FinishRunStepParams{ID: step.ID, EnterpriseID: run.EnterpriseID, Status: status}); stepErr != nil {
		return "", 0, stepErr
	}
	if err != nil {
		return "", 0, err
	}
	summary := strings.TrimSpace(output.String())
	if summary == "" {
		return "", 0, fmt.Errorf("empty compaction summary")
	}
	if usage.OutputSource() != "provider" {
		return summary, 0, nil
	}
	return summary, int(usage.Output), nil
}

func ChooseCompactionBoundary(events []db.ConversationEvent) (through, firstKept int64, ok bool) {
	if len(events) < 4 {
		return 0, 0, false
	}
	total, lastUser := 0, int64(0)
	for _, event := range events {
		total += max(len(event.Payload), 1)
		if event.EventType == "user_message" {
			lastUser = event.Sequence
		}
	}
	pending := map[string]bool{}
	consumed := 0
	for index, event := range events {
		consumed += max(len(event.Payload), 1)
		var payload struct {
			Calls []modelprovider.ToolCall `json:"tool_calls"`
			ID    string                   `json:"tool_call_id"`
		}
		if len(event.Payload) > 0 && json.Unmarshal(event.Payload, &payload) != nil {
			return 0, 0, false
		}
		if event.EventType == "assistant_message" {
			for _, call := range payload.Calls {
				pending[call.ID] = true
			}
		}
		if event.EventType == "tool_call_result" {
			delete(pending, payload.ID)
		}
		if index+1 >= len(events) || event.Sequence >= lastUser || len(pending) != 0 {
			continue
		}
		if event.EventType == "assistant_message" && len(payload.Calls) == 0 || event.EventType == "model_usage" {
			through, firstKept, ok = event.Sequence, events[index+1].Sequence, true
			if consumed >= total/2 {
				return through, firstKept, true
			}
		}
	}
	return
}

func deterministicSummary(events []db.ConversationEvent) string {
	var output strings.Builder
	for _, event := range events {
		if event.EventType != "user_message" && event.EventType != "assistant_message" {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		content, _ := payload["content"].(string)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(event.EventType)
		output.WriteString(": ")
		output.WriteString(content)
		if output.Len() > 8192 {
			break
		}
	}
	value := output.String()
	if len(value) > 8192 {
		value = value[:8192]
	}
	return value
}

func (compactor Compactor) fail(ctx context.Context, run db.Run, errorCode string) error {
	return fencedRunTransition(ctx, compactor.Store, func(q *db.Queries) error {
		current, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		if terminalRun(current.Status) {
			return nil
		}
		if current.Status != "waiting_system" || current.StopReason.String != "context_compaction" {
			return nil
		}
		_, err = conversation.FinishRunRecord(ctx, q, current, "failed", "context_compaction_failed", errorCode, "system", "")
		return err
	})
}
