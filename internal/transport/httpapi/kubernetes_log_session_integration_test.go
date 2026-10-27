package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/config"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type httpSessionLogBackend struct {
	reads int
	raw   []byte
}

func (b *httpSessionLogBackend) ListInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	return kubernetesinventory.Page{}, kubernetesinventory.ErrUnsupported
}
func (b *httpSessionLogBackend) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	if q.Type != "pod" || q.Namespace != "orders" || q.NamespaceUID != "orders-uid" || q.Name != "api" || q.UID != "pod-uid" {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
	}
	p := corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-uid", ResourceVersion: "1"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://main"}}}}
	raw, _ := json.Marshal(p)
	var u unstructured.Unstructured
	_ = u.UnmarshalJSON(raw)
	d, _ := kubernetesinventory.Lookup("pod")
	return kubernetesinventory.Project(d, u, q.NamespaceUID, time.Now())
}
func (b *httpSessionLogBackend) ReadPinnedPodLogs(_ context.Context, _ db.KubernetesCluster, _ uuid.UUID, q kubernetespodlogs.Query) (kubernetespodlogs.Batch, error) {
	b.reads++
	return kubernetespodlogs.Continue(b.raw, q, time.Now())
}

func TestPostgresPodLogSessionHTTPAuthenticatesTicketsCSRFAndScope(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	enterprise, department, user, role, cluster, namespace := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 log HTTP',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Log operator')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Namespace log operator')", role, enterprise)
	for id, description := range authorization.PermissionRegistry {
		exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.read'),($1,'kubernetes.operational_logs.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'log target','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, enterprise)
	_, err = store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: namespace, EnterpriseID: enterprise, ClusterID: cluster, Name: "orders", NamespaceUid: "orders-uid"})
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), enterprise, user, namespace)
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	tokens := []string{uuid.NewString(), uuid.NewString()}
	httpIDs := []uuid.UUID{uuid.New(), uuid.New()}
	csrf := uuid.NewString()
	now := time.Now().UTC()
	stamp := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	for i, token := range tokens {
		_, err = store.Queries.CreateSession(ctx, db.CreateSessionParams{ID: httpIDs[i], TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash(csrf), Audience: "enterprise", UserID: user, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: department, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		_, _ = store.Pool.Exec(context.Background(), "DELETE FROM kubernetes_log_sessions WHERE enterprise_id=$1", enterprise)
		for _, id := range httpIDs {
			_, _ = store.Pool.Exec(context.Background(), "DELETE FROM sessions WHERE id=$1", id)
		}
	}()
	backend := &httpSessionLogBackend{raw: []byte(now.Add(-time.Second).Format(time.RFC3339Nano) + " native-http-log token=http-private-canary\n")}
	var router http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) }))
	defer server.Close()
	handler := KubernetesHandler{Identity: EnterpriseIdentityHandler{Auth: SetupHandler{Config: config.Server{AllowedOrigins: []string{server.URL}}, Identity: identity.Service{Store: store}}}, Workbench: kubernetesworkbench.Service{Store: store, Backend: backend, CursorKey: bytes.Repeat([]byte{9}, 32)}}
	router = NewRouterWithOptions(RouterOptions{Kubernetes: &handler, AllowedOrigins: []string{server.URL}})
	base := "/api/v1/enterprise/kubernetes-clusters/" + cluster.String() + "/log-sessions"
	call := func(method, path string, body any, token, csrfValue string) (int, []byte, http.Header) {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req, e := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", server.URL)
		req.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: token})
		req.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: csrf})
		if csrfValue != "" {
			req.Header.Set("X-CSRF-Token", csrfValue)
		}
		response, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, raw, response.Header
	}
	input := map[string]any{"namespace_id": namespace, "pod": "api", "pod_uid": "pod-uid", "container": "main", "previous": false, "tail_lines": 100, "since_seconds": 300}
	if status, _, _ := call("POST", base, input, tokens[0], "wrong"); status != 403 {
		t.Fatalf("CSRF status %d", status)
	}
	status, raw, headers := call("POST", base, input, tokens[0], csrf)
	if status != 201 || headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("create status %d", status)
	}
	if strings.Contains(string(raw), "continuity_key") || strings.Contains(string(raw), "ticket_hash") {
		t.Fatal("private binding exposed")
	}
	var ticket api.KubernetesLogSessionTicket
	if json.Unmarshal(raw, &ticket) != nil {
		t.Fatal("ticket response")
	}
	path := base + "/" + ticket.Session.Id.String()
	if status, _, _ = call("POST", path+"/attach", map[string]string{"ticket": ticket.Ticket}, tokens[1], csrf); status != 403 {
		t.Fatal("ticket moved across HTTP sessions", status)
	}
	if status, _, _ = call("POST", path+"/attach", map[string]string{"ticket": ticket.Ticket}, tokens[0], csrf); status != 200 {
		t.Fatal("ticket attach", status)
	}
	if status, _, _ = call("POST", path+"/attach", map[string]string{"ticket": ticket.Ticket}, tokens[0], csrf); status < 400 {
		t.Fatal("ticket replay succeeded")
	}
	status, raw, headers = call("POST", path+"/read", map[string]any{}, tokens[0], csrf)
	if status != 200 || headers.Get("Cache-Control") != "no-store" || strings.Contains(string(raw), "http-private-canary") || !strings.Contains(string(raw), "REDACTED") {
		t.Fatal("read privacy or status", status)
	}
	if status, _, headers = call("GET", path, nil, tokens[0], ""); status != 200 || headers.Get("Cache-Control") != "no-store" {
		t.Fatal("paused authorization check", status)
	}
	reads := backend.reads
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", enterprise, namespace)
	if status, raw, _ = call("GET", path, nil, tokens[0], ""); status != 403 || strings.Contains(string(raw), "native-http-log") {
		t.Fatal("paused session did not revoke", status)
	}
	if status, raw, _ = call("POST", path+"/read", map[string]any{}, tokens[0], csrf); status != 403 || backend.reads != reads || strings.Contains(string(raw), "native-http-log") {
		t.Fatal("revoked API session read native logs", status)
	}
	t.Log("actual HTTP cookie/CSRF/one-use ticket/header/scope workflow passed with controlled log backend")
}
