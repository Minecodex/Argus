package argusdev

import (
	"encoding/json"
	"errors"
	"strings"
)

func plan11CustomerMCPTraceAllowed(raw, readAlias string) error {
	var calls []struct{ Tool string }
	if json.Unmarshal([]byte(raw), &calls) != nil || len(calls) == 0 || !strings.HasPrefix(readAlias, "mcp_") {
		return errors.New("customer MCP trace or connection-bound identity missing")
	}
	for _, call := range calls {
		if call.Tool != readAlias && call.Tool != "tool.search" && call.Tool != "tool.describe" {
			return errPlan11NavigationBoundary
		}
	}
	return nil
}
