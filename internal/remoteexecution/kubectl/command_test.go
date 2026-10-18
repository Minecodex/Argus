package kubectl

import (
	"context"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPinnedConfigCannotForwardCredentialsOrChangeConnection(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Impersonate-User") != "" {
			t.Error("credentials escaped")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	original := &rest.Config{Host: "https://other.invalid", BearerToken: "private-token", Username: "user", Password: "password", Impersonate: rest.ImpersonationConfig{UserName: "admin"}, TLSClientConfig: rest.TLSClientConfig{KeyData: []byte("private key")}}
	cfg := pinnedConfig(original, u, false)
	if cfg.Host != server.URL || cfg.BearerToken != "" || cfg.Password != "" || cfg.ExecProvider != nil || cfg.Impersonate.UserName != "" || len(cfg.KeyData) != 0 {
		t.Fatal("connection configuration not replaced")
	}
	transport, err := rest.TransportFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{server.URL, "https://other.invalid"} {
		r, _ := http.NewRequestWithContext(context.Background(), "GET", endpoint, nil)
		result, e := transport.RoundTrip(r)
		if endpoint == server.URL {
			if e != nil || result.StatusCode != 200 {
				t.Fatal("approved proxy failed", e)
			}
			result.Body.Close()
		} else if e == nil {
			t.Fatal("connection override reached a server")
		}
	}
	if calls != 1 {
		t.Fatal("unexpected request count", calls)
	}
	rejected, err := rest.TransportFor(pinnedConfig(original, u, true))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest("GET", server.URL, nil)
	if _, err = rejected.RoundTrip(r); err == nil {
		t.Fatal("explicit override accepted")
	}
}
func TestProxyEndpointMustBeLocalAndCredentialFree(t *testing.T) {
	for _, value := range []string{"https://127.0.0.1:1234", "http://example.test:1234", "http://user:password@127.0.0.1:1234", "http://127.0.0.1:1234/path"} {
		if _, err := proxyURL(value); err == nil {
			t.Errorf("accepted %s", value)
		}
	}
}
