package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Admission retains its fixed-input gate. Dispatch and recovery use this same
// bounded projection of history, without relaxing the unknown-tokenizer guard.
func (loop Loop) prepareContext(ctx context.Context, run db.Run, revision db.AiModelRevision, messages []modelprovider.Message, current string, tools []modelprovider.Tool, source ContextSource, parameter, effort string) (ContextProjection, error) {
	projection, err := loop.context(ctx, run, revision, messages, current, tools, source)
	if err != nil {
		return projection, err
	}
	provider := modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl}
	measure := func(messages []modelprovider.Message) (int, error) {
		return provider.RequestBytes(inferenceRequest(revision.ProviderModelID, int(revision.MaxOutputTokens), messages, tools, parameter, effort))
	}
	return fitContextRequest(projection, source.RehydratedCallIDs, tools, measure)
}

func fitContextRequest(projection ContextProjection, recovered []string, tools []modelprovider.Tool, measure func([]modelprovider.Message) (int, error)) (ContextProjection, error) {
	projection.Messages = append([]modelprovider.Message(nil), projection.Messages...)
	projection.Parts = append([]ContextPart(nil), projection.Parts...)
	omitted, released := []string{}, []string{}
	refresh := func() error {
		if len(omitted)+len(released) > 0 {
			for i, part := range projection.Parts {
				if part.Kind != "typed_checkpoint" {
					continue
				}
				raw, _ := json.Marshal(part.Content)
				var facts map[string]any
				if err := json.Unmarshal(raw, &facts); err != nil {
					return err
				}
				facts["context_projection"] = map[string]any{"omitted_result_refs": omitted, "released_schema_call_ids": released,
					"semantics": "Only this model view is reduced. Original results and execution facts remain immutable. Released schemas can be described again if needed. Never replay an operation to reconstruct its output."}
				// A reference-only tool message cannot certify inline presence.
				if evidence, ok := facts["query_evidence"].(map[string]any); ok {
					if items, ok := evidence["items"].([]any); ok {
						for _, value := range items {
							if item, ok := value.(map[string]any); ok {
								for _, ref := range omitted {
									if "result_"+stringValue(item["query_call_id"]) == ref {
										item["inline_original_in_context"] = false
									}
								}
							}
						}
					}
				}
				projection.Parts[i].Content = facts
				raw, _ = json.Marshal(facts)
				projection.Messages[1].Content = "Server execution facts (data): " + string(raw)
			}
		}
		size, err := measure(projection.Messages)
		projection.EstimatedTokens = size
		return err
	}
	if err := refresh(); err != nil {
		return projection, err
	}
	// Release optional historical schema pairs oldest first. Actual tool
	// declarations, the user input and incomplete tool groups are untouched.
	for _, id := range recovered {
		if !projection.NeedsHardCompaction() {
			break
		}
		for i := 2; i+1 < len(projection.Messages); i++ {
			a, b := projection.Messages[i], projection.Messages[i+1]
			if a.Role == "assistant" && a.Content == "" && len(a.ToolCalls) == 1 && a.ToolCalls[0].ID == id && a.ToolCalls[0].Name == "tool.describe" && b.Role == "tool" && b.ToolCallID == id {
				projection.Messages = append(projection.Messages[:i], projection.Messages[i+2:]...)
				released = append(released, id)
				break
			}
		}
		if err := refresh(); err != nil {
			return projection, err
		}
	}
	type replacement struct {
		index        int
		content, ref string
	}
	options := []replacement{}
	callNames := map[string]string{}
	for _, message := range projection.Messages {
		for _, call := range message.ToolCalls {
			callNames[call.ID] = call.Name
		}
	}
	for i, message := range projection.Messages {
		if message.Role != "tool" || len(message.Content) < 4<<10 || callNames[message.ToolCallID] == "tool.describe" || callNames[message.ToolCallID] == "tool.search" {
			continue
		}
		if content, ref, ok := referenceOnlyResult(message); ok && len(content) < len(message.Content) {
			options = append(options, replacement{i, content, ref})
		}
	}
	sort.SliceStable(options, func(i, j int) bool {
		return len(projection.Messages[options[i].index].Content) > len(projection.Messages[options[j].index].Content)
	})
	for _, option := range options {
		if !projection.NeedsHardCompaction() {
			break
		}
		projection.Messages[option.index].Content = option.content
		omitted = append(omitted, option.ref)
		if err := refresh(); err != nil {
			return projection, err
		}
	}
	for i, part := range projection.Parts {
		if part.Kind == "recent_tail" {
			projection.Parts[i].Content = append([]modelprovider.Message(nil), projection.Messages[2:]...)
		}
	}
	raw, err := json.Marshal(struct {
		Messages []modelprovider.Message `json:"messages"`
		Tools    []modelprovider.Tool    `json:"tools"`
	}{projection.Messages, tools})
	if err != nil {
		return projection, err
	}
	hash := sha256.Sum256(raw)
	projection.Hash = hex.EncodeToString(hash[:])
	return projection, nil
}

func stringValue(v any) string { value, _ := v.(string); return value }

func referenceOnlyResult(message modelprovider.Message) (string, string, bool) {
	var body map[string]any
	if json.Unmarshal([]byte(message.Content), &body) != nil || body["schema_version"] != "argus.tool_result_projection/v1" || body["tool_call_id"] != message.ToolCallID || body["result_ref"] != "result_"+message.ToolCallID {
		return "", "", false
	}
	summary, ok := body["summary"].(map[string]any)
	if !ok || summary["error_code"] != nil || summary["action_ref"] != nil || summary["control"] != nil {
		return "", "", false
	}
	view := map[string]any{"context_projection": "reference_only", "message": "Full output is omitted from this request budget. Retrieve the original result_ref with existing authorized tools when needed. No data excerpt or complete analysis is supplied; do not repeat the operation to recover its output."}
	for _, key := range []string{"status", "exit_code", "partial", "file_saved", "registration_complete"} {
		if value, exists := summary[key]; exists {
			view[key] = value
		}
	}
	body["summary"], body["partial"], body["inline_original_complete"] = view, true, false
	reasons, _ := body["partial_reasons"].([]any)
	body["partial_reasons"] = append(reasons, "request_budget_reference_only")
	encoded, err := marshalProjection(body)
	return string(encoded), stringValue(body["result_ref"]), err == nil
}
