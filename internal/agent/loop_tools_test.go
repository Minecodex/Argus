package agent

import (
	"testing"
)

func TestToolExecutionRejectsTruncatedModelStops(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"length", "max_tokens", "incomplete", "content_filter", "failed", "cancelled", "", "unknown"} {
		if allowsToolExecution(reason) {
			t.Fatalf("tool execution allowed for stop reason %q", reason)
		}
	}
	for _, reason := range []string{"stop", "tool_calls", "completed"} {
		if !allowsToolExecution(reason) {
			t.Fatalf("tool execution rejected for complete stop reason %q", reason)
		}
	}
}
