package agent

import (
	"errors"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func finalInferenceTurn(turn int) bool { return turn == toolruntime.MaxInferenceTurns-1 }

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
