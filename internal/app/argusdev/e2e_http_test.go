package argusdev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScenarioHTTPFailureEvidenceRedactsCredentials(t *testing.T) {
	for _, body := range []string{
		`{"code":"FIXTURE_FAILURE","api_token":"sensitive-fixture","nested":{"password":"sensitive-fixture"}}`,
		`{"api_key":"sensitive-fixture","nested":{"credential_value":"sensitive-fixture","apiKey":"sensitive-fixture"}}`,
		`upstream error: sensitive-fixture`,
	} {
		t.Run(body[:8], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			artifacts := t.TempDir()
			client := NewScenarioHTTP(server.URL, artifacts, nil)
			_, err := client.JSON(context.Background(), "failure", "", http.MethodGet, "/", http.StatusOK, nil, nil)
			if err == nil || strings.Contains(err.Error(), "sensitive-fixture") {
				t.Fatalf("unexpected failure message: %v", err)
			}
			evidence, err := os.ReadFile(filepath.Join(artifacts, "failure-response.json"))
			if err != nil || strings.Contains(string(evidence), "sensitive-fixture") {
				t.Fatalf("evidence was not safely saved: %v", err)
			}
		})
	}
}
