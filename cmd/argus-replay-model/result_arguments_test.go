//go:build m4e2e

package main

import "testing"

func TestReplayResultArgumentsUsesObservedIdentities(t *testing.T) {
	for _, m := range []message{{Role: "tool", Content: `{"summary":{"id":"actual-runtime-id"}}`}, {Type: "function_call_output", Output: `{"summary":{"id":"actual-runtime-id"}}`}} {
		args := map[string]any{"arguments": map[string]any{"id": map[string]any{"$result_pointer": "/0/summary/id"}}}
		result, err := replayResultArguments(args, []message{m})
		if err != nil || result["arguments"].(map[string]any)["id"] != "actual-runtime-id" {
			t.Fatal(result, err)
		}
		if _, err := replayResultArguments(args, nil); err == nil {
			t.Fatal("invented absent result")
		}
	}
}
