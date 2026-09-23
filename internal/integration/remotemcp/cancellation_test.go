package remotemcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cancellationTransport struct {
	base    http.RoundTripper
	entered chan struct{}
	method  string
}

func (r cancellationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(req)
	if err == nil && (r.method == "" || r.method == req.Method) && (response.Header.Get("Content-Type") == "text/event-stream" || response.Header.Get("Content-Type") == "application/json") {
		response.Body = &cancellationBody{ReadCloser: response.Body, entered: r.entered}
	}
	return response, err
}

func TestCancellationDuringSSERecoveryKeepsOriginalIdentityWithoutReplay(t *testing.T) {
	var calls, resumes, cancellations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			resumes.Add(1)
			if r.Header.Get("Last-Event-ID") != "resume-1" {
				t.Error("resume cursor changed")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": recovery waiting\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		var req struct {
			Method string `json:"method"`
			Params struct {
				RequestID int `json:"requestId"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "tools/call" {
			calls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "id: resume-1\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
		} else if req.Method == "notifications/cancelled" {
			if req.Params.RequestID != 1 || r.Header.Get("Mcp-Session-Id") != "resume-session" {
				t.Error("cancellation lost original request identity")
			}
			cancellations.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()
	reading := make(chan struct{})
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := Client{Endpoint: server.URL, SessionID: "resume-session", HTTP: &http.Client{Transport: cancellationTransport{base: transport, entered: reading, method: http.MethodGet}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.Call(ctx, "write_once", map[string]any{}); done <- err }()
	select {
	case <-reading:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery body was not read")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled write reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("recovery did not cancel")
	}
	if calls.Load() != 1 || resumes.Load() != 1 || cancellations.Load() != 1 {
		t.Fatalf("calls=%d resumes=%d cancellations=%d", calls.Load(), resumes.Load(), cancellations.Load())
	}
}

type cancellationBody struct {
	io.ReadCloser
	once    sync.Once
	entered chan struct{}
}

func (r *cancellationBody) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	return r.ReadCloser.Read(p)
}

func TestCancellationAfterResponseHeaders(t *testing.T) {
	for _, media := range []string{"text/event-stream", "application/json"} {
		t.Run(media, func(t *testing.T) { testCancellationAfterResponseHeaders(t, media) })
	}
}

func testCancellationAfterResponseHeaders(t *testing.T, media string) {
	var cancellations, calls atomic.Int32
	notified := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
			Params struct {
				RequestID json.RawMessage `json:"requestId"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		switch request.Method {
		case "tools/call":
			calls.Add(1)
			w.Header().Set("Content-Type", media)
			if media == "text/event-stream" {
				_, _ = io.WriteString(w, ": waiting for a long tool operation\n\n")
			} else {
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","result":`)
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "notifications/cancelled":
			if r.Header.Get("Mcp-Session-Id") != "review-session" || len(request.Params.RequestID) == 0 {
				t.Error("cancellation lost request/session identity")
			}
			cancellations.Add(1)
			notified <- struct{}{}
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	reading := make(chan struct{})
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := Client{Endpoint: server.URL, HTTP: &http.Client{Transport: cancellationTransport{base: transport, entered: reading}}, SessionID: "review-session"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := client.Call(ctx, "long_task", map[string]any{}); finished <- err }()
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("SSE body was not entered")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled invocation succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not unblock the call")
	}
	select {
	case <-notified:
	case <-time.After(250 * time.Millisecond):
	}
	if calls.Load() != 1 {
		t.Fatalf("business call was repeated: %d", calls.Load())
	}
	if cancellations.Load() != 1 {
		t.Fatalf("expected one cancellation notification after SSE response headers; got %d", cancellations.Load())
	}
}
