package modelprovider

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTerminalModelEventDoesNotWaitForTransportEOF(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol Protocol
		stream   string
		want     error
	}{
		{"chat_with_separate_usage", ProtocolChatCompletions, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n", nil},
		{"responses_completed", ProtocolResponses, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":4,\"output_tokens\":1}}}\n\n", nil},
		{"premature_done", ProtocolChatCompletions, "data: [DONE]\n\n", ErrIncompleteStream},
		{"invalid_usage", ProtocolChatCompletions, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1,\"prompt_tokens_details\":{\"cached_tokens\":9}}}\n\ndata: [DONE]\n\n", ErrInvalidUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tc.stream)
				w.(http.Flusher).Flush()
				// The protocol is finished, but a proxy/provider holds HTTP open.
				<-r.Context().Done()
			}))
			defer server.Close()
			provider := Provider{Protocol: tc.protocol, BaseURL: server.URL, Client: server.Client(), StreamTimeout: time.Second}
			var usage TokenUsage
			err := provider.Stream(t.Context(), Request{Model: "terminal-event", MaxTokens: 16}, func(e Event) error { usage.Observe(e); return nil })
			if !errors.Is(err, tc.want) {
				t.Fatalf("terminal event waited for transport or lost validation: %v", err)
			}
			if tc.want == nil && (!usage.Complete() || usage.Input != 4 || usage.Output != 1) {
				t.Fatal("terminal handling dropped separate provider usage")
			}
		})
	}
}
