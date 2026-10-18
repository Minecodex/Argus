package argusdev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStatusPollingRecoversTransientReadAndPreservesAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/runs/fixture" {
			t.Errorf("unexpected observation %s %s", r.Method, r.URL.Path)
		}
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("private-upstream-body"))
			return
		}
		_, _ = w.Write([]byte(`{"status":"succeeded"}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	client := NewScenarioHTTP(server.URL, directory, nil)
	value, err := client.PollJSON(t.Context(), "observe", "", "/runs/fixture?secret=private-query", map[string]string{"Authorization": "Bearer private-header"})
	if err != nil || value["status"] != "succeeded" || calls.Load() != 3 {
		t.Fatalf("read did not recover within its bound: %v %v calls=%d", value, err, calls.Load())
	}
	attempts, err := filepath.Glob(filepath.Join(directory, "*-retry.json"))
	if err != nil || len(attempts) != 2 {
		t.Fatalf("missing retry evidence: %v %v", attempts, err)
	}
	files, _ := filepath.Glob(filepath.Join(directory, "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil || strings.Contains(string(raw), "private-") {
			t.Fatalf("read retry leaked private data: %s %v", file, err)
		}
	}
}

func TestStatusPollingDoesNotRetryMutationsDenialsOrMalformedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		status             int
		poll               bool
		calls              int32
	}{
		{"approval", http.MethodPost, `{}`, 502, false, 1},
		{"forbidden", http.MethodGet, `{}`, 403, true, 1},
		{"rate-limited", http.MethodGet, `{}`, 429, true, 1},
		{"backend-error", http.MethodGet, `{}`, 500, true, 1},
		{"malformed", http.MethodGet, `not-json`, 200, true, 1},
		{"permanent-unavailable", http.MethodGet, `{}`, 503, true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != tc.method {
					t.Errorf("method changed to %s", r.Method)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewScenarioHTTP(server.URL, t.TempDir(), nil)
			var err error
			if tc.poll {
				_, err = client.PollJSON(t.Context(), "observe", "", "/runs/fixture", nil)
			} else {
				_, err = client.JSON(t.Context(), "confirm", "", tc.method, "/confirm", 200, nil, nil)
			}
			if err == nil || calls.Load() != tc.calls {
				t.Fatalf("unexpected acceptance/replay: err=%v calls=%d", err, calls.Load())
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := NewScenarioHTTP("http://127.0.0.1:1", t.TempDir(), nil)
	if _, err := client.PollJSON(ctx, "cancelled", "", "/runs/fixture", nil); err == nil {
		t.Fatal("cancelled polling accepted")
	}
}
