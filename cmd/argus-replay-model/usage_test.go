//go:build m4e2e

package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

func TestMissingUsageFixtureForBothProtocols(t *testing.T) {
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		for _, missing := range []bool{false, true} {
			var server *httptest.Server
			if protocol == modelprovider.ProtocolResponses {
				server = httptest.NewServer(http.HandlerFunc(responses))
			} else {
				server = httptest.NewServer(http.HandlerFunc(chatCompletions))
			}
			prompt := "ordinary summary"
			if missing {
				prompt = "argus_e2e_usage_missing"
			}
			var usage modelprovider.TokenUsage
			err := (modelprovider.Provider{Protocol: protocol, BaseURL: server.URL}).Stream(t.Context(), modelprovider.Request{Model: "replay", MaxTokens: 100, Messages: []modelprovider.Message{{Role: "user", Content: prompt}}}, func(e modelprovider.Event) error { usage.Observe(e); return nil })
			server.Close()
			if err != nil || usage.Complete() == missing {
				t.Fatalf("%s missing=%v usage=%+v error=%v", protocol, missing, usage, err)
			}
		}
	}
}

func TestContradictoryUsageFixtureIncludesCompletedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(chatCompletions))
	defer server.Close()
	plan := base64.RawURLEncoding.EncodeToString([]byte(`[{"tool":"tool.search","arguments":{"category":"host","query":"list"}}]`))
	var usage modelprovider.TokenUsage
	completedCalls := 0
	err := (modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: server.URL}).Stream(t.Context(), modelprovider.Request{Model: "replay", MaxTokens: 100, Messages: []modelprovider.Message{{Role: "user", Content: "argus_e2e_usage_inconsistent argus_e2e_plan_b64:" + plan}}, Tools: []modelprovider.Tool{{Name: "tool.search", Schema: map[string]any{"type": "object"}}}}, func(e modelprovider.Event) error {
		usage.Observe(e)
		if e.Type == "tool_call_done" {
			completedCalls++
		}
		return nil
	})
	if !errors.Is(err, modelprovider.ErrInvalidUsage) || completedCalls != 1 || usage.Input != 50 || usage.CachedInput != 80 || usage.Complete() {
		t.Fatalf("err=%v calls=%d usage=%+v", err, completedCalls, usage)
	}
}
