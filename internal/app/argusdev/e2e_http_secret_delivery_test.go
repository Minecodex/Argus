package argusdev

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScenarioHTTPSecretDeliveryStaysOutOfEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"private-key-canary","ticket":"private-ticket-canary","nonce":"private-nonce-canary","key_id":"public-key-id","ticket_expires_at":"2026-10-21T12:00:00Z"}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	client := NewScenarioHTTP(server.URL, directory, nil)
	response, err := client.JSON(t.Context(), "secret-delivery", "", http.MethodPost, "/", 201, nil, nil)
	if err == nil || strings.Contains(err.Error(), "private-") {
		t.Fatal("unexpected-status error exposed private delivery")
	}
	if response["key"] != "private-key-canary" || response["ticket"] != "private-ticket-canary" {
		t.Fatal("authorized in-memory delivery was changed")
	}
	evidence, err := os.ReadFile(filepath.Join(directory, "secret-delivery-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-key-canary", "private-ticket-canary", "private-nonce-canary"} {
		if strings.Contains(string(evidence), secret) {
			t.Fatal("private delivery persisted in evidence")
		}
	}
	if !strings.Contains(string(evidence), "public-key-id") || !strings.Contains(string(evidence), "ticket_expires_at") {
		t.Fatal("public diagnostic metadata was removed")
	}
}
