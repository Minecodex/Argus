package kubernetesstreamgateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestWebSocketOriginHandshake(t *testing.T) {
	server := httptest.NewServer(WebSocketGateway{
		Service:    kubernetesworkbench.Service{Store: &postgres.Store{}},
		InstanceID: "origin-test",
		Backends:   Factory{},
		AllowedOrigins: []string{
			"https://portal.example.test",
			"https://trailing.example.test/",
			"https://[::1]:8443",
		},
	})
	t.Cleanup(server.Close)
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/kubernetes/sessions/" + uuid.NewString()
	for _, tc := range []struct {
		name, origin string
		allowed      bool
	}{
		{"cli", "", true},
		{"same host", server.URL, true},
		{"configured", "https://portal.example.test", true},
		{"case insensitive", "https://PORTAL.example.test", true},
		{"trailing configuration slash", "https://trailing.example.test", true},
		{"ipv6", "https://[::1]:8443", true},
		{"unconfigured", "https://untrusted.example.test", false},
		{"different scheme", "http://portal.example.test", false},
		{"different port", "https://portal.example.test:8443", false},
		{"host suffix", "https://portal.example.test.attacker.test", false},
		{"userinfo", "https://user@portal.example.test", false},
		{"path", "https://portal.example.test/path", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, response, err := websocket.Dial(t.Context(), endpoint, &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{tc.origin}},
			})
			if conn != nil {
				defer conn.CloseNow()
			}
			if tc.allowed {
				if err != nil {
					t.Fatalf("authorized origin rejected: %v", err)
				}
			} else if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
				t.Fatalf("unauthorized origin was not rejected with 403: response=%v err=%v", response, err)
			}
		})
	}
}
