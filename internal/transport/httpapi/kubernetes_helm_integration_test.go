package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type httpHelmBackend struct {
	httpSessionLogBackend
	service  *kubernetesworkbench.Service
	previews int
}

func (b *httpHelmBackend) HelmReleases(_ context.Context, _ db.KubernetesCluster, r kuberneteshelm.Request) ([]kuberneteshelm.Release, error) {
	return []kuberneteshelm.Release{{Name: "sample", Namespace: r.Namespace.Name, Revision: 1, Status: "deployed", Chart: "sample", ChartVersion: "1.0.0", UpdatedAt: time.Now().UTC()}}, nil
}
func (b *httpHelmBackend) PreviewHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	b.previews++
	m, e := b.service.Credentials.LoadHelmInput(ctx, c.EnterpriseID, c.ID, r, true, "server", c.ID.String())
	if e != nil {
		return kuberneteshelm.Result{}, e
	}
	defer func() { clear(m.Chart); clear(m.Values); clear(m.Key) }()
	if r.Source != nil {
		if len(m.Chart) != 0 || m.SourceCredentials == nil || m.SourceCredentials.Password != "http-source-private-canary" {
			return kuberneteshelm.Result{}, kuberneteshelm.ErrInvalid
		}
		return kuberneteshelm.Result{}, kuberneteshelm.ErrSourceAuth
	}
	if string(m.Chart) != "http-protected-chart-canary" || string(m.Values) != "password: http-protected-values-canary" {
		return kuberneteshelm.Result{}, kuberneteshelm.ErrInvalid
	}
	d, _ := kubernetesinventory.Lookup("configmap")
	return kuberneteshelm.Result{SchemaVersion: kuberneteshelm.SchemaVersion, DryRun: true, Objects: []kuberneteshelm.Object{{Definition: d, Namespace: r.Namespace.Name, NamespaceUID: r.Namespace.UID, Name: "settings", Action: "apply", Validation: "server", Changes: []string{"/data/*"}}}, SealedPlan: []byte("controlled-native-seal")}, nil
}
func (b *httpHelmBackend) ExecuteHelm(context.Context, db.KubernetesCluster, kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return kuberneteshelm.Result{}, kuberneteshelm.ErrUnavailable
}
func (b *httpHelmBackend) ObserveHelm(context.Context, db.KubernetesCluster, kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return kuberneteshelm.Result{}, kuberneteshelm.ErrUnavailable
}

func TestPostgresHelmHTTPProtectedInputsCSRFReplayAndNamespaceScope(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	if e := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "..", "migrations", "postgresql"), postgres.MigrationUp); e != nil {
		t.Fatal(e)
	}
	store, e := postgres.Open(ctx, address)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	enterprise, department, user, role, cluster, namespace, connector := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 Helm HTTP',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Helm operator')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Namespace Helm operator')", role, enterprise)
	for id, description := range authorization.PermissionRegistry {
		exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.read'),($1,'kubernetes.helm.manage'),($1,'kubernetes.configuration.manage')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Helm target','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, enterprise)
	if _, e = store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: namespace, EnterpriseID: enterprise, ClusterID: cluster, Name: "orders", NamespaceUid: "orders-uid"}); e != nil {
		t.Fatal(e)
	}
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), enterprise, user, namespace)
	exec("INSERT INTO connectors(id,enterprise_id,role,name,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at,capabilities) VALUES($1,$2,'kubernetes','helm-http',$3,$4,decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'online',1,now()+interval '1 hour',$5)", connector, enterprise, cluster, connector.String(), []string{kuberneteshelm.SchemaVersion})
	exec("UPDATE kubernetes_clusters SET connector_id=$2 WHERE id=$1", cluster, connector)
	exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at) VALUES($1,$2,'gateway',1,$3,now(),now())", connector, enterprise, []string{kuberneteshelm.SchemaVersion})
	if e = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); e != nil {
		t.Fatal(e)
	}
	key := bytes.Repeat([]byte{8}, 32)
	keyfile := filepath.Join(t.TempDir(), "keyring.json")
	encoded, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(key)}})
	if e = os.WriteFile(keyfile, encoded, 0600); e != nil {
		t.Fatal(e)
	}
	keyring, e := secret.LoadKeyring(keyfile)
	if e != nil {
		t.Fatal(e)
	}
	csrf, token, httpID := uuid.NewString(), uuid.NewString(), uuid.New()
	now := time.Now().UTC()
	stamp := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	_, e = store.Queries.CreateSession(ctx, db.CreateSessionParams{ID: httpID, TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash(csrf), Audience: "enterprise", UserID: user, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: department, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = store.Pool.Exec(context.Background(), "DELETE FROM sessions WHERE id=$1", httpID) })
	backend := &httpHelmBackend{}
	domain := kubernetesworkbench.Service{Store: store, Backend: backend, CursorKey: key, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}, Credentials: secret.Service{Store: store, Keyring: keyring}}
	backend.service = &domain
	var router http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) }))
	defer server.Close()
	handler := KubernetesHandler{Identity: EnterpriseIdentityHandler{Auth: SetupHandler{Config: config.Server{AllowedOrigins: []string{server.URL}}, Identity: identity.Service{Store: store}}}, Workbench: domain}
	router = NewRouterWithOptions(RouterOptions{Kubernetes: &handler, AllowedOrigins: []string{server.URL}})
	base := "/api/v1/enterprise/kubernetes-clusters/" + cluster.String() + "/helm"
	call := func(method, path string, body any, csrfValue, key string) (int, []byte, http.Header) {
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
		r.Header.Set("X-CSRF-Token", csrfValue)
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: token})
		r.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: csrf})
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data, response.Header
	}
	input := map[string]any{"namespace_id": namespace, "operation": "install", "name": "sample", "expected_revision": 0, "timeout_seconds": 120, "reason": "Validate owned HTTP flow", "chart_archive": base64.StdEncoding.EncodeToString([]byte("http-protected-chart-canary")), "values": "password: http-protected-values-canary"}
	idempotency := uuid.NewString()
	if status, _, _ := call("POST", base+"/actions/preview", input, uuid.NewString(), idempotency); status != 403 {
		t.Fatal("CSRF", status)
	}
	status, raw, headers := call("POST", base+"/actions/preview", input, csrf, idempotency)
	if status != 201 || headers.Get("Cache-Control") != "no-store" {
		t.Fatal("protected preview", status, string(raw))
	}
	for _, private := range []string{"http-protected-values-canary", "http-protected-chart-canary", "sealed_plan", "private_input_id", "immutable_plan"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private HTTP response", private)
		}
	}
	if status, _, _ := call("POST", base+"/actions/preview", input, csrf, idempotency); status != 201 || backend.previews != 1 {
		t.Fatal("HTTP replay rendered again", status, backend.previews)
	}
	if status, _, _ := call("GET", base+"/releases?namespace_id="+namespace.String(), nil, "", ""); status != 200 {
		t.Fatal("release list", status)
	}
	if status, _, _ := call("GET", base+"/releases/sample/history?namespace_id="+namespace.String(), nil, "", ""); status != 200 {
		t.Fatal("release history", status)
	}
	if status, _, _ := call("GET", base+"/releases?namespace_id="+uuid.NewString(), nil, "", ""); status != 403 {
		t.Fatal("Namespace escape", status)
	}
	sourceInput := map[string]any{"namespace_id": namespace, "operation": "install", "name": "remote-source", "expected_revision": 0, "timeout_seconds": 120, "reason": "Verify protected origin authentication", "source": map[string]any{"kind": "repository", "url": "https://charts.example/team", "chart": "sample", "version": "1.0.0"}, "source_credentials": map[string]string{"username": "reader", "password": "http-source-private-canary"}}
	if status, body, _ := call("POST", base+"/actions/preview", sourceInput, csrf, uuid.NewString()); status != 422 || strings.Contains(string(body), "http-source-private-canary") || !strings.Contains(string(body), "KUBERNETES_HELM_SOURCE_AUTH_FAILED") {
		t.Fatal("source credential delivery or safe authentication failure", status, string(body))
	}
	var staged int
	if e := store.Pool.QueryRow(ctx, "SELECT count(*) FROM kubernetes_helm_inputs WHERE enterprise_id=$1 AND status='staged'", enterprise).Scan(&staged); e != nil || staged != 0 {
		t.Fatal("failed source preview left staged credentials", e, staged)
	}
	exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='kubernetes.configuration.manage'", role)
	if status, _, _ := call("POST", base+"/actions/preview", input, csrf, uuid.NewString()); status != 403 {
		t.Fatal("object permission bypass", status)
	}
	t.Log("actual HTTP cookie/CSRF/protected input/replay/list/history/Namespace scope passed with controlled native adapter")
}
