package modelprovider

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestReasoningProfileIsExplicitAndProtocolBound(t *testing.T) {
	for _, fixture := range []struct {
		protocol     Protocol
		capabilities string
		effort       string
		invalid      bool
	}{
		{ProtocolChatCompletions, `{}`, "", false},
		{ProtocolChatCompletions, `{"reasoning_effort":"low"}`, "low", false},
		{ProtocolChatCompletions, `{"reasoning_effort":"high"}`, "high", false},
		{ProtocolChatCompletions, `{"reasoning_effort":"max"}`, "max", false},
		{ProtocolChatCompletions, `{"reasoning_effort":"arbitrary"}`, "", true},
		{ProtocolChatCompletions, `{"reasoning_effort":23}`, "", true},
		{ProtocolResponses, `{"reasoning_effort":"low"}`, "", true},
	} {
		effort, err := ReasoningEffortForCapabilities(fixture.protocol, []byte(fixture.capabilities))
		if effort != fixture.effort || fixture.invalid != errors.Is(err, ErrBudgetProfileInvalid) {
			t.Fatalf("profile %s: effort=%q err=%v", fixture.capabilities, effort, err)
		}
	}
	for _, effort := range []string{"", "provider_default", "low", "high", "max"} {
		_, wire, err := (Provider{Protocol: ProtocolChatCompletions, BaseURL: "https://example.test/v1"}).buildRequest(Request{Model: "fixture", MaxTokens: 128, ReasoningEffort: effort})
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if json.Unmarshal(wire, &body) != nil {
			t.Fatal("invalid request")
		}
		if effort == "" || effort == "provider_default" {
			if _, exists := body["reasoning_effort"]; exists {
				t.Fatal("default reasoning was overridden")
			}
		} else if body["reasoning_effort"] != effort {
			t.Fatal("requested reasoning setting lost")
		}
	}
}
