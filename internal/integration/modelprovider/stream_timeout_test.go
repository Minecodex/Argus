package modelprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamingResponseUsesSeparateBoundedDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for range 3 {
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"part\"}}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 20 * time.Millisecond
	provider := Provider{Protocol: ProtocolChatCompletions, BaseURL: server.URL, Client: client, StreamTimeout: 2 * time.Second}
	var usage TokenUsage
	if err := provider.Stream(t.Context(), Request{Model: "bounded", MaxTokens: 32}, func(event Event) error { usage.Observe(event); return nil }); err != nil {
		t.Fatal(err)
	}
	if !usage.Complete() || client.Timeout != 20*time.Millisecond {
		t.Fatal("stream lost final provider usage or mutated the shared HTTP client")
	}
}

func TestStreamingDeadlineAndCancellationStillAbortRead(t *testing.T) {
	for _, explicitCancel := range []bool{false, true} {
		t.Run(fmt.Sprint(explicitCancel), func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			provider := Provider{Protocol: ProtocolChatCompletions, BaseURL: server.URL, Client: server.Client(), StreamTimeout: 60 * time.Millisecond}
			if explicitCancel {
				provider.StreamTimeout = time.Second
				go func() { <-started; cancel() }()
			}
			begin := time.Now()
			err := provider.Stream(ctx, Request{Model: "bounded", MaxTokens: 32}, func(Event) error { return nil })
			want := context.DeadlineExceeded
			if explicitCancel {
				want = context.Canceled
			}
			if !errors.Is(err, want) || time.Since(begin) > time.Second {
				t.Fatalf("stream ignored its deadline/cancellation: %v after %s", err, time.Since(begin))
			}
		})
	}
}

func TestEndpointPolicyRetainsBoundedConnectionHeadersAndOrdinaryRequests(t *testing.T) {
	client := (PublicEndpointPolicy{}).Client()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.ResponseHeaderTimeout != DefaultStreamTimeout || client.Timeout != 2*time.Minute || transport.DialContext == nil {
		t.Fatal("stream support removed the normal endpoint timeout/proxy policy")
	}
}

func TestStreamingPrefillAndBodyShareTheBoundedDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 20 * time.Millisecond
	transport := client.Transport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = (PublicEndpointPolicy{}).Client().Transport.(*http.Transport).ResponseHeaderTimeout
	client.Transport = transport
	defer transport.CloseIdleConnections()
	provider := Provider{Protocol: ProtocolChatCompletions, BaseURL: server.URL, Client: client, StreamTimeout: time.Second}
	if err := provider.Stream(t.Context(), Request{Model: "delayed-prefill", MaxTokens: 32}, func(Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	provider.StreamTimeout = 30 * time.Millisecond
	if err := provider.Stream(t.Context(), Request{Model: "delayed-prefill", MaxTokens: 32}, func(Event) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("prefill ignored the stream deadline: %v", err)
	}
}
