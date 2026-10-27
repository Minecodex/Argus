package toolruntime

// ModelSystemInstructions is the initial instruction prefix shared by message
// admission and inference. Later turns update only the server-owned budget.
func ModelSystemInstructions(mode string) string {
	return ModelSystemInstructionsForTurn(mode, 0)
}
