package toolruntime

import "fmt"

// MaxInferenceTurns is the durable per-Run inference limit. Compaction is
// separately accounted; worker recovery never resets this counter.
const MaxInferenceTurns = 24

func ModelSystemInstructionsForTurn(mode string, completed int) string {
	completed = min(max(completed, 0), MaxInferenceTurns)
	remaining := MaxInferenceTurns - completed
	budget := fmt.Sprintf("\n\nRun inference budget: %d of %d calls have been used; %d remain including this call. The final call is reserved for a response with no tools. Complete necessary evidence gathering early; batch independent reads and avoid rereading unchanged files. Once sufficient authorized evidence is available, answer the user instead of continuing to inspect or reformat. Preserve full identifiers and exact observed enum values in structured JSON; put explanations in separate permitted text fields. Never invent missing measurements, verification, or completed actions.", completed, MaxInferenceTurns, remaining)
	if remaining == 1 {
		budget += " This is the final response call. No tools are available. Answer using only the currently authorized evidence already present. Clearly distinguish verified results from unfinished work and unknown outcomes; do not promise execution after this response or claim an unverified change succeeded."
	} else if remaining <= 4 {
		budget += " The budget is nearly exhausted. Use only indispensable remaining reads, then provide the requested conclusion and limitations."
	}
	return SystemInstructions + BuiltinToolInstructions + RemoteInstructions + RemoteModeInstructions(mode) + budget
}
