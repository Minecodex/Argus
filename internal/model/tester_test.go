package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

type publicTestResolver struct{}

func (publicTestResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCompatibilityFreezesOnlyAnEnforcedOutputParameter(t *testing.T) {
	for _, tc := range []struct {
		name, expected             string
		rejectLegacy, ignoreLegacy bool
	}{
		{"legacy", modelprovider.BudgetMaxTokens, false, false},
		{"modern rejects legacy", modelprovider.BudgetMaxCompletionTokens, true, false},
		{"silently ignored legacy", modelprovider.BudgetMaxCompletionTokens, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := []string{}
			client := &http.Client{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["reasoning_effort"] != "low" {
					t.Fatal("a compatibility probe lost the requested reasoning profile")
				}
				parameter := modelprovider.BudgetMaxCompletionTokens
				if _, ok := body[modelprovider.BudgetMaxTokens]; ok {
					parameter = modelprovider.BudgetMaxTokens
				}
				selected = append(selected, parameter)
				if tc.rejectLegacy && parameter == modelprovider.BudgetMaxTokens {
					return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("private provider response")), Header: http.Header{}}, nil
				}
				messages, _ := json.Marshal(body["messages"])
				finish, text, output := "stop", "OK", 8
				delta := map[string]any{}
				if strings.Contains(string(messages), modelprovider.OutputBudgetProbeMarker) {
					output = int(body[parameter].(float64))
					finish = "length"
					if tc.ignoreLegacy && parameter == modelprovider.BudgetMaxTokens {
						output += 500
						finish = "stop"
					}
				} else if _, ok := body["tools"]; ok {
					finish = "tool_calls"
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "compat-1", "type": "function", "function": map[string]any{"name": "compatibility_probe", "arguments": "{}"}}}
				} else if _, ok := body["response_format"]; ok {
					text = `{"ok":true}`
				}
				if finish == "stop" {
					delta["content"] = text
				}
				wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 32, "completion_tokens": output}})
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(wire) + "\n\ndata: [DONE]\n\n"))}, nil
			})}
			input := Input{BaseURL: "https://public.example/v1", ProviderModelID: "model", Protocol: "chat_completions", MaxOutput: 8192, ContextWindow: 200000, ReasoningEffort: "low"}
			result := (ProviderTester{Policy: modelprovider.PublicEndpointPolicy{Resolver: publicTestResolver{}}, Client: client}).Test(t.Context(), input)
			if !result.Compatible || result.OutputTokenParameter != tc.expected {
				t.Fatalf("wrong measured profile: %+v", result)
			}
			capabilities := capabilitiesJSON(input, result)
			if effort, err := modelprovider.ReasoningEffortForCapabilities(modelprovider.ProtocolChatCompletions, capabilities); err != nil || effort != "low" {
				t.Fatal("revision did not freeze the explicit reasoning profile")
			}
			parameter, err := modelprovider.BudgetParameterForCapabilities(modelprovider.ProtocolChatCompletions, capabilities)
			if err != nil || parameter != tc.expected {
				t.Fatalf("detected profile lost before revision storage: %s %v", parameter, err)
			}
			for _, actual := range selected[len(selected)-3:] {
				if actual != tc.expected {
					t.Fatal("subsequent probes did not use the selected parameter")
				}
			}
		})
	}
}

func TestCompatibilityDoesNotSwitchParametersAfterTransportOrAuthorizationFailure(t *testing.T) {
	for _, status := range []int{0, 401, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				calls++
				if status == 0 {
					return nil, &net.DNSError{IsTimeout: true, Err: "private provider endpoint"}
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("private"))}, nil
			})}
			result := (ProviderTester{Policy: modelprovider.PublicEndpointPolicy{Resolver: publicTestResolver{}}, Client: client}).Test(t.Context(), Input{BaseURL: "https://public.example", ProviderModelID: "model", Protocol: "chat_completions", MaxOutput: 8192})
			if result.Compatible || calls != 1 {
				t.Fatalf("unexpected fallback/acceptance: %d %+v", calls, result)
			}
		})
	}
}
