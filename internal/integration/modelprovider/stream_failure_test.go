package modelprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamFailureDiagnosticsUseBoundedClasses(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind string
	}{
		{context.DeadlineExceeded, "deadline"},
		{fmt.Errorf("wrapped: %w", ErrIncompleteStream), "incomplete_stream"},
		{ErrInvalidUsage, "invalid_usage"},
		{io.ErrUnexpectedEOF, "unexpected_eof"},
		{fmt.Errorf("private wrapper: %w", errors.New("model returned an incomplete tool call")), "incomplete_tool_call"},
		{errors.New("model tool calls lack tool_calls finish reason"), "invalid_tool_finish_reason"},
		{errors.New("https://secret.example?key=private-secret-value"), "unclassified_stream_or_sink_error"},
	} {
		if got := streamFailureKind(tc.err); got != tc.kind {
			t.Errorf("wrong bounded failure class: %s", got)
		}
	}
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: private-response-value\n\n")
	}))
	defer server.Close()
	provider := Provider{Protocol: ProtocolChatCompletions, BaseURL: server.URL, APIKey: "private-secret-value", Client: server.Client()}
	if err := provider.Stream(t.Context(), Request{Model: "diagnostic", MaxTokens: 16}, func(Event) error { return nil }); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	text := output.String()
	if !strings.Contains(text, `"failure_kind":"invalid_json"`) {
		t.Fatal("safe failure class missing")
	}
	for _, private := range []string{server.URL, "private-secret-value", "private-response-value"} {
		if strings.Contains(text, private) {
			t.Fatal("provider/private content leaked into diagnostics")
		}
	}
}
