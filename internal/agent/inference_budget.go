package agent

import (
	"errors"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func finalInferenceTurn(turn int) bool { return turn == toolruntime.MaxInferenceTurns-1 }

// Dispatch and hard-compaction recovery must measure this same wire
// request, including provider capability fields. Otherwise recovery can resume
// a request that the inference worker immediately rejects as oversized again.
func inferenceRequest(model string, maxTokens int, messages []modelprovider.Message, tools []modelprovider.Tool, budgetParameter, reasoningEffort string) modelprovider.Request {
	return modelprovider.Request{Model: model, MaxTokens: maxTokens, Messages: messages, Tools: tools,
		OutputTokenParameter: budgetParameter, ReasoningEffort: reasoningEffort}
}

func inferenceTurnContext(mode string, turn int, messages []modelprovider.Message, tools []modelprovider.Tool) ([]modelprovider.Message, []modelprovider.Tool, error) {
	if turn < 0 || turn >= toolruntime.MaxInferenceTurns || len(messages) == 0 || messages[0].Role != "system" {
		return nil, nil, errors.New("invalid bounded inference context")
	}
	current := append([]modelprovider.Message(nil), messages...)
	current[0].Content = toolruntime.ModelSystemInstructionsForTurn(mode, turn)
	if finalInferenceTurn(turn) {
		tools = nil
	}
	return current, tools, nil
}
