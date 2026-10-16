package alerting

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestCurrentAndHistoricalEventToolStateContractsCannotMisrepresentHistory(t *testing.T) {
	for _, history := range []bool{false, true} {
		schema, err := toolruntime.CompileInputSchema(eventQueryToolSchema(history))
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range []string{"normal", "pending", "firing", "recovering", "resolved", "closed"} {
			err := schema.Validate(map[string]any{"lifecycle": state})
			allowed := history == (state == "resolved" || state == "closed")
			if (err == nil) != allowed {
				t.Fatalf("history=%v lifecycle=%s error=%v", history, state, err)
			}
		}
		if err := schema.Validate(map[string]any{}); err != nil {
			t.Fatal("unfiltered query rejected", err)
		}
	}
	current := alertDiscovery("alert.event.query")
	history := alertDiscovery("alert.history.query")
	if !strings.Contains(current.Description, "history.query") || !strings.Contains(current.Examples[0].Guidance, "resolved/closed") || !strings.Contains(history.Description, "resolved or closed") {
		t.Fatal("discovery does not distinguish current absence from recovery history")
	}
}
