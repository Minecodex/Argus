package argusdev

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPIReadinessDoesNotAcceptStaticPortalOrUnavailableUpstream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/setup/status" {
			t.Errorf("unexpected readiness request %s %s", r.Method, r.URL.Path)
		}
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadGateway)
		case 2:
			_, _ = w.Write([]byte("ready\n"))
		default:
			_, _ = w.Write([]byte(`{"state":"uninitialized"}`))
		}
	}))
	defer server.Close()
	if err := waitHTTPSReady(t.Context(), server.Client(), server.URL+"/api/v1/setup/status", `"state":`, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("API readiness skipped the upstream result")
	}
}
