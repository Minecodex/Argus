package modelprovider

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

type diagnosticDatabaseError struct{}

func (diagnosticDatabaseError) Error() string    { return "private query and credential" }
func (diagnosticDatabaseError) SQLState() string { return "40001" }

func TestStreamReadAndSinkFailuresKeepTheirCauseAndOrigin(t *testing.T) {
	provider := Provider{Protocol: ProtocolChatCompletions}
	wire := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"private content\"}}]}\n\n"
	readFailure := errors.New("private transport endpoint")
	err := provider.consumeSSE(io.MultiReader(strings.NewReader(wire), iotest.ErrReader(readFailure)), func(Event) error { return nil })
	if !errors.Is(err, readFailure) || streamFailureOrigin(err) != "response_read" {
		t.Fatalf("read failure lost its cause or origin: %T", err)
	}
	sinkFailure := &diagnosticDatabaseError{}
	err = provider.consumeSSE(strings.NewReader(wire), func(Event) error { return sinkFailure })
	if !errors.Is(err, sinkFailure) || streamFailureOrigin(err) != "event_sink" || streamFailureType(err) != "database_serialization" {
		t.Fatalf("sink failure lost its cause or safe diagnostic: %T", err)
	}
}

func TestStreamSinkDiagnosticsNeverLogPrivateErrorText(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"private response\"}}]}\n\n")
	}))
	defer server.Close()
	provider := Provider{Protocol: ProtocolChatCompletions, BaseURL: server.URL, APIKey: "private key", Client: server.Client()}
	err := provider.Stream(t.Context(), Request{Model: "diagnostic", MaxTokens: 16}, func(Event) error { return &diagnosticDatabaseError{} })
	var database *diagnosticDatabaseError
	if !errors.As(err, &database) {
		t.Fatal("caller can no longer inspect the original error")
	}
	text := output.String()
	for _, expected := range []string{`"failure_origin":"event_sink"`, `"failure_type":"database_serialization"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("safe diagnostic absent: %s", expected)
		}
	}
	for _, private := range []string{server.URL, "private key", "private response", "private query and credential"} {
		if strings.Contains(text, private) {
			t.Fatal("private data leaked into stream diagnostics")
		}
	}
}
