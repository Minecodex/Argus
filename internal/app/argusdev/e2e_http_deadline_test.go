package argusdev

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCompatibilityDeadlinePreservesSessionAndOrdinaryTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("test-session"); err != nil || cookie.Value != "owned" {
			http.Error(w, "missing session", 403)
			return
		}
		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte(`{"compatible":true}`))
	}))
	defer server.Close()
	h := NewScenarioHTTP(server.URL, t.TempDir(), http.DefaultTransport)
	client := h.Client("enterprise")
	client.Timeout = 5 * time.Millisecond
	u, _ := url.Parse(server.URL)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: "test-session", Value: "owned"}})
	bounded := h.withClientTimeout("enterprise", time.Second)
	if _, err := bounded.JSON(t.Context(), "compatibility", "enterprise", http.MethodGet, "/", 200, nil, nil); err != nil {
		t.Fatal(err)
	}
	if client.Timeout != 5*time.Millisecond || bounded.Client("enterprise").Jar != client.Jar {
		t.Fatal("prerequisite changed the ordinary deadline or authenticated session")
	}
}
