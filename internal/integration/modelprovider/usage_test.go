package modelprovider

import (
	"encoding/json"
	"testing"
)

func TestUsageProvenancePreservesMissingPartialAndReportedZero(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolChatCompletions, ProtocolResponses} {
		for _, test := range []struct {
			name, fields, inputSource, outputSource string
			input, output                           int64
		}{
			{"missing", `{}`, "estimated", "estimated", 40, 9},
			{"partial", `{"INPUT":17}`, "provider", "estimated", 17, 9},
			{"zero", `{"INPUT":0,"OUTPUT":0}`, "provider", "provider", 0, 0},
			{"complete", `{"INPUT":17,"OUTPUT":6}`, "provider", "provider", 17, 6},
		} {
			t.Run(string(protocol)+"/"+test.name, func(t *testing.T) {
				var fields map[string]any
				if err := json.Unmarshal([]byte(test.fields), &fields); err != nil {
					t.Fatal(err)
				}
				usage := map[string]any{}
				input, output := "prompt_tokens", "completion_tokens"
				if protocol == ProtocolResponses {
					input, output = "input_tokens", "output_tokens"
				}
				if v, ok := fields["INPUT"]; ok {
					usage[input] = v
				}
				if v, ok := fields["OUTPUT"]; ok {
					usage[output] = v
				}
				message := map[string]any{"choices": []any{}, "usage": usage}
				if protocol == ProtocolResponses {
					message = map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "usage": usage}}
				}
				data, _ := json.Marshal(message)
				var value TokenUsage
				if err := (Provider{Protocol: protocol}).decodeEvent(data, func(event Event) error { value.Observe(event); return nil }); err != nil {
					t.Fatal(err)
				}
				value.Estimate(40, 9)
				if value.Input != test.input || value.Output != test.output || value.InputSource() != test.inputSource || value.OutputSource() != test.outputSource {
					t.Fatalf("usage=%+v sources=%s/%s", value, value.InputSource(), value.OutputSource())
				}
			})
		}
	}
}

func TestStreamingRequestsAlwaysAskForUsage(t *testing.T) {
	for _, tools := range [][]Tool{nil, {{Name: "test", Schema: map[string]any{"type": "object"}}}} {
		_, data, err := (Provider{Protocol: ProtocolChatCompletions, BaseURL: "https://example.test"}).buildRequest(Request{Model: "test", MaxTokens: 100, Tools: tools, Messages: []Message{{Role: "user", Content: "Summarize"}}})
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Options struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		if err := json.Unmarshal(data, &body); err != nil || !body.Options.IncludeUsage {
			t.Fatalf("usage missing: %s (%v)", data, err)
		}
	}
}
