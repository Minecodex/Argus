package opensandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCommandStreamDistinguishesTerminalFailuresFromMissingResults(t *testing.T) {
	for _, scenario := range []struct {
		name, events string
		exit         int
		unknown      bool
	}{
		{"success", `{"type":"status","text":"running"}` + "\n" + `{"type":"stdout","text":"analysis-complete\n"}` + "\n" + `{"type":"execution_complete","execution_time":12}`, 0, false},
		{"process failure is terminal", `{"type":"stderr","text":"script failed\n"}` + "\n" + `{"type":"error","error":{"ename":"CommandExecError","evalue":"7"}}`, 7, false},
		{"start failure is terminal", `{"type":"error","error":{"ename":"CommandExecError","evalue":"process start failed"}}`, 1, false},
		{"truncated stream", `{"type":"stdout","text":"partial"}`, 0, true},
		{"invalid error", `{"type":"error","error":null}`, 0, true},
		{"unknown protocol event", `{"type":"unexpected"}`, 0, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var endpoint string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/endpoints/") {
					_ = json.NewEncoder(w).Encode(Endpoint{Endpoint: endpoint, Headers: map[string]string{"X-EXECD-ACCESS-TOKEN": "fixture-only"}})
					return
				}
				if r.URL.Path != "/command" || r.Header.Get("X-EXECD-ACCESS-TOKEN") != "fixture-only" {
					t.Error("command endpoint identity lost")
				}
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request["timeout"] != float64(60000) || request["cwd"] != "/workspace" || request["background"] != false {
					t.Error("command request contract differs")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "{\"type\":\"init\",\"text\":\"command-id\"}\n\n")
				for _, event := range strings.Split(scenario.events, "\n") {
					_, _ = fmt.Fprintf(w, "%s\n\n", event)
				}
			}))
			defer server.Close()
			endpoint = server.URL
			client, err := NewClient(server.URL, "fixture-only")
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.RunCommand(context.Background(), "sandbox", "echo test", 60)
			if (err != nil) != scenario.unknown || !scenario.unknown && result.ExitCode != scenario.exit {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestPinnedExecdCommandStream(t *testing.T) {
	endpoint := os.Getenv("ARGUS_P5_EXECD_URL")
	if endpoint == "" {
		t.Skip("requires the task-owned Execd v1.0.22 probe container")
	}
	lifecycle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Endpoint{Endpoint: endpoint})
	}))
	defer lifecycle.Close()
	client, err := NewClient(lifecycle.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		command, output string
		exit            int
	}{
		{`python -c 'import pandas as pd; print(pd.Series([1,2,3]).sum())'`, "6", 0},
		{`echo known-error >&2; exit 7`, "", 7},
	} {
		result, err := client.RunCommand(t.Context(), "probe", scenario.command, 60)
		if err != nil || result.ExitCode != scenario.exit || strings.TrimSpace(result.Stdout) != scenario.output {
			t.Fatalf("actual Execd stream: result=%+v error=%v", result, err)
		}
	}
}
