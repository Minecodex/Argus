package agent

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type toolContext struct {
	Facts    map[string]any
	Messages []modelprovider.Message
}

// Restore only native discovery/contracts, never business effects or arbitrary
// external results. The persisted gateway still enforces the actual Run chain.
func (loop Loop) toolContextFacts(ctx context.Context, run db.Run, through, compacted int64, scope string) (toolContext, error) {
	rows, err := loop.Store.Queries.ListRunToolContextRecords(ctx, db.ListRunToolContextRecordsParams{
		EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, ThroughSequence: through, AuthorizationScope: scope})
	result := toolContext{Facts: toolruntime.EmptyDiscoveryFacts(run.ID.String())}
	result.Facts["complete"] = len(rows) <= 32
	if err != nil {
		return result, err
	}
	searches, descriptions, recentErrors, recentResults := []map[string]any{}, []map[string]any{}, []map[string]any{}, []map[string]any{}
	seenResults := map[string]bool{}
	resultBytes := 0
	seen := map[string]bool{}
	used := 0
	for _, row := range rows[:min(len(rows), 32)] {
		var payload struct {
			ContextFacts map[string]any  `json:"context_facts"`
			ToolID       string          `json:"tool_id"`
			Target       string          `json:"target_tool_id"`
			Status       string          `json:"status"`
			Projection   json.RawMessage `json:"projection"`
			ResultRef    string          `json:"result_ref"`
		}
		if json.Unmarshal(row.Payload, &payload) != nil {
			continue
		}
		var input struct {
			Category string `json:"category"`
			Name     string `json:"name"`
		}
		if json.Unmarshal(row.Input, &input) != nil {
			continue
		}
		var projection struct {
			Summary map[string]any `json:"summary"`
		}
		if json.Unmarshal(payload.Projection, &projection) != nil {
			continue
		}
		if payload.Status == "succeeded" && payload.ContextFacts != nil {
			kind, kindOK := payload.ContextFacts["kind"].(string)
			id, idOK := payload.ContextFacts["id"].(string)
			key := kind + "/" + id
			raw, _ := json.Marshal(payload.ContextFacts)
			if kindOK && idOK && kind != "" && id != "" && !seenResults[key] {
				// A newer record that exceeds the budget must not expose an older
				// version of that object's state as its current fact.
				seenResults[key] = true
				if len(recentResults) >= 8 || len(raw) > 12<<10 || resultBytes+len(raw) > 16<<10 {
					result.Facts["complete"] = false
				} else {
					recentResults = append(recentResults, map[string]any{"source_sequence": row.Sequence, "result_ref": payload.ResultRef, "tool": payload.Target, "facts": payload.ContextFacts})
					resultBytes += len(raw)
				}
			}
		}
		if payload.Status == "failed" {
			if len(recentErrors) < 4 {
				recentErrors = append(recentErrors, map[string]any{"tool": payload.Target, "category": input.Category, "name": input.Name,
					"error_code": projection.Summary["error_code"], "message": projection.Summary["message"], "details": projection.Summary["details"], "source_sequence": row.Sequence})
			}
			continue
		}
		key := payload.ToolID + "/" + input.Category + "/" + input.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		if payload.ToolID == "tool.search" {
			searches = append(searches, map[string]any{"category": input.Category, "result_ref": payload.ResultRef, "source_sequence": row.Sequence})
		} else if payload.ToolID == "tool.describe" {
			descriptions = append(descriptions, map[string]any{"category": input.Category, "name": input.Name,
				"version": projection.Summary["version"], "result_ref": payload.ResultRef, "source_sequence": row.Sequence})
			if row.Sequence <= compacted && len(result.Messages) < 8 {
				call := modelprovider.ToolCall{ID: row.ToolCallID.String(), Name: "tool.describe", Arguments: string(row.Input)}
				pair := []modelprovider.Message{{Role: "assistant", ToolCalls: []modelprovider.ToolCall{call}},
					{Role: "tool", ToolCallID: call.ID, Content: string(payload.Projection)}}
				raw, _ := json.Marshal(pair)
				if used+len(raw) <= 24<<10 {
					result.Messages = append(pair, result.Messages...)
					used += len(raw)
				}
			}
		}
	}
	result.Facts["searched_categories"], result.Facts["described_tools"], result.Facts["recent_errors"] = searches, descriptions, recentErrors
	result.Facts["recent_results"] = recentResults
	return result, nil
}
