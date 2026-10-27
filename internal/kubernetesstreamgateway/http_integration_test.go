package kubernetesstreamgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/transport/httpapi"
)

func TestPostgresKubernetesStreamHTTPIdentityCSRFAndGatewayAttach(t *testing.T) {
	f := newGatewayFixture(t)
	ctx := t.Context()
	csrf := uuid.NewString()
	tokens := []string{uuid.NewString(), uuid.NewString()}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for i, id := range ids {
		f.exec("INSERT INTO sessions(id,token_hash,csrf_hash,audience,user_id,enterprise_id,department_id,authorization_version,locale,idle_expires_at,absolute_expires_at,last_seen_at) SELECT $1,$2,$3,audience,user_id,enterprise_id,department_id,authorization_version,locale,idle_expires_at,absolute_expires_at,last_seen_at FROM sessions WHERE id=$4", id, identity.TokenHash(tokens[i]), identity.TokenHash(csrf), f.http)
	}
	t.Cleanup(func() {
		_, _ = f.service.Store.Pool.Exec(context.Background(), "DELETE FROM kubernetes_stream_sessions WHERE enterprise_id=$1", f.actor.EnterpriseID)
		for _, id := range ids {
			_, _ = f.service.Store.Pool.Exec(context.Background(), "DELETE FROM sessions WHERE id=$1", id)
		}
	})
	var router http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) }))
	defer server.Close()
	handler := httpapi.KubernetesHandler{Identity: httpapi.EnterpriseIdentityHandler{Auth: httpapi.SetupHandler{Config: config.Server{AllowedOrigins: []string{server.URL}}, Identity: identity.Service{Store: f.service.Store}}}, Workbench: f.service, WebsocketURL: "wss://gateway.example.test"}
	mux := http.NewServeMux()
	mux.Handle("/v1/kubernetes/sessions/", WebSocketGateway{Service: f.service, InstanceID: "gateway-a", Backends: f.factory})
	mux.Handle("/", httpapi.NewRouterWithOptions(httpapi.RouterOptions{Kubernetes: &handler, AllowedOrigins: []string{server.URL}}))
	router = mux
	base := "/api/v1/enterprise/kubernetes-clusters/" + f.cluster.String() + "/stream-sessions"
	call := func(method, path string, body any, token, csrfValue string) (int, []byte, http.Header) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r, e := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", server.URL)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "argus_enterprise_session", Value: token})
		}
		r.AddCookie(&http.Cookie{Name: "argus_enterprise_csrf", Value: csrf})
		if csrfValue != "" {
			r.Header.Set("X-CSRF-Token", csrfValue)
		}
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e = io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, raw, response.Header
	}
	input := kubernetesworkbench.StreamInput{NamespaceID: f.namespace, Type: "pod", Name: "api", UID: "pod-uid", Mode: "exec", Shell: "sh", Container: "main", Reason: "HTTP gateway integration"}
	if status, _, _ := call("POST", base, input, tokens[0], "wrong"); status != 403 {
		t.Fatal("CSRF bypass", status)
	}
	if status, _, _ := call("POST", base, input, "", csrf); status < 400 {
		t.Fatal("anonymous stream creation")
	}
	f.exec("DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM role_bindings WHERE enterprise_id=$1) AND permission_id='kubernetes.exec'", f.actor.EnterpriseID)
	if status, _, _ := call("POST", base, input, tokens[0], csrf); status != 403 {
		t.Fatal("read permission implied terminal", status)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'kubernetes.exec' FROM role_bindings WHERE enterprise_id=$1", f.actor.EnterpriseID)
	status, raw, headers := call("POST", base, input, tokens[0], csrf)
	if status != 201 || headers.Get("Cache-Control") != "no-store" {
		t.Fatal("create", status, string(raw))
	}
	var ticket kubernetesworkbench.StreamTicket
	if json.Unmarshal(raw, &ticket) != nil || strings.Contains(string(raw), "binding") || strings.Contains(string(raw), "credential_lease") {
		t.Fatal("ticket contract exposed private binding")
	}
	path := base + "/" + ticket.Session.ID.String()
	if status, _, _ := call("GET", path, nil, tokens[1], ""); status != 403 {
		t.Fatal("stream moved across login sessions", status)
	}
	status, raw, headers = call("POST", path+"/ticket", nil, tokens[0], csrf)
	if status != 200 || headers.Get("Cache-Control") != "no-store" {
		t.Fatal("ticket replace", status)
	}
	if json.Unmarshal(raw, &ticket) != nil {
		t.Fatal("ticket contract")
	}
	ws, _, e := websocket.Dial(ctx, ticket.WebsocketURL, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.CloseNow()
	hello, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "client_hello", Sequence: 1, Ticket: ticket.Ticket, Cols: 100, Rows: 30})
	if e = ws.Write(ctx, websocket.MessageText, hello); e != nil {
		t.Fatal(e)
	}
	_, raw, e = ws.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var ready ServerFrame
	if json.Unmarshal(raw, &ready) != nil || ready.Type != "server_ready" {
		t.Fatal("HTTP ticket did not attach", string(raw))
	}
	if status, raw, headers = call("GET", path, nil, tokens[0], ""); status != 200 || headers.Get("Cache-Control") != "no-store" || !bytes.Contains(raw, []byte(`"status":"active"`)) {
		t.Fatal("active HTTP state", status, string(raw))
	}
	if status, _, _ := call("POST", path+"/close", nil, tokens[0], csrf); status != 200 {
		t.Fatal("close", status)
	}
	_, raw, e = ws.Read(ctx)
	if e != nil {
		t.Fatal("close state", e)
	}
	var end ServerFrame
	if json.Unmarshal(raw, &end) != nil || end.Status != "closed" {
		t.Fatal("close state", string(raw))
	}
	if status, raw, _ = call("GET", path, nil, tokens[0], ""); status != 200 || !bytes.Contains(raw, []byte(`"status":"closed"`)) {
		t.Fatal("final HTTP state", status, string(raw))
	}
}
