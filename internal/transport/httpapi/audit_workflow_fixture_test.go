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
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/connector"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

type auditWorkflowFixture struct {
	t                                  *testing.T
	store                              *postgres.Store
	server                             *httptest.Server
	enterprise, user, department, host uuid.UUID
	token, csrf                        string
	domain                             resource.Service
	workflow                           actionservice.Service
	executor                           actionservice.Executor
}

func newAuditWorkflowFixture(t *testing.T) *auditWorkflowFixture {
	t.Helper()
	store := postgrestest.New(t, "ARGUS_AUDIT_TEST_DATABASE_URL", "audit_workflow", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	f := &auditWorkflowFixture{t: t, store: store, enterprise: uuid.New(), user: uuid.New(), department: uuid.New(), host: uuid.New()}
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Audit acceptance',$2,'Asia/Shanghai')", f.enterprise, f.enterprise.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", f.department, f.enterprise)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'audit-operator','Audit operator')", f.user, f.enterprise, f.department)
	role := uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Audit acceptance operator')", role, f.enterprise)
	for id, description := range authorization.PermissionRegistry {
		f.exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.enterprise, f.user, role)
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'audit-target','192.0.2.10',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", f.host, f.enterprise)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.enterprise, f.user, f.host)
	if err := audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{23}, 32)
	f.domain = resource.Service{Store: store, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	f.workflow = actionservice.Service{Store: store, Resources: f.domain, Idempotency: f.domain.Actions.Idempotency, OneTimeResultKey: key}
	f.executor = actionservice.Executor{Store: store, Resources: f.domain, OneTimeResultKey: key}
	var router http.Handler
	dist := filepath.Join("..", "..", "..", "web", "apps", "enterprise", "dist")
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("ARGUS_AUDIT_BUILT_UI") != "1" || strings.HasPrefix(r.URL.Path, "/api/") {
			router.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/argus-runtime.json" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"templateOrigin": f.server.URL})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.FileServer(http.Dir(dist)).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	}))
	t.Cleanup(f.server.Close)
	origins := []string{f.server.URL, "http://127.0.0.1:4273"}
	auth := SetupHandler{Config: config.Server{AllowedOrigins: origins}, Identity: identity.Service{Store: store}}
	enterprise := EnterpriseIdentityHandler{Auth: auth, Service: identity.EnterpriseService{Store: store}, Cursor: pagination.Signer{Key: key}}
	workflow := WorkflowHandler{Identity: enterprise, Service: f.workflow}
	actions := ResourceActionHandler{Identity: enterprise, Service: f.domain, Workflow: f.workflow}
	hosts := HostHandler{Identity: enterprise, Service: f.domain, Queries: store.Queries}
	auditHandler := AuditHandler{Auth: auth, Enterprise: enterprise, Store: store, Cursor: pagination.Signer{Key: key}}
	clusters := KubernetesHandler{Identity: enterprise, Service: f.domain}
	connectors := ConnectorHandler{Identity: enterprise, Service: connector.Service{Store: store}, Bastion: connector.BastionService{Store: store}, Queries: store.Queries}
	router = NewRouterWithOptions(RouterOptions{Setup: &auth, EnterpriseIdentity: &enterprise, ResourceAction: &actions, Workflow: &workflow, Host: &hosts, Kubernetes: &clusters, Connector: &connectors, Audit: &auditHandler, AllowedOrigins: origins})
	f.session()
	return f
}

func (f *auditWorkflowFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *auditWorkflowFixture) session() {
	f.t.Helper()
	f.token, f.csrf = uuid.NewString(), uuid.NewString()
	user, err := f.store.Queries.GetEnterpriseUser(f.t.Context(), db.GetEnterpriseUserParams{ID: f.user, EnterpriseID: f.enterprise})
	if err != nil {
		f.t.Fatal(err)
	}
	now := time.Now().UTC()
	stamp := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	_, err = f.store.Queries.CreateSession(f.t.Context(), db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(f.token), CsrfHash: identity.TokenHash(f.csrf), Audience: "enterprise", UserID: f.user, EnterpriseID: uuid.NullUUID{UUID: f.enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: f.department, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: user.AuthorizationVersion, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if err != nil {
		f.t.Fatal(err)
	}
}
func (f *auditWorkflowFixture) call(method, path string, body any, want int, key string) []byte {
	f.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	request, err := http.NewRequestWithContext(f.t.Context(), method, f.server.URL+"/api/v1"+path, bytes.NewReader(raw))
	if err != nil {
		f.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", f.server.URL)
	request.Header.Set("X-CSRF-Token", f.csrf)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: f.token})
	request.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: f.csrf})
	response, err := f.server.Client().Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	if response.StatusCode != want {
		f.t.Fatalf("%s %s status %d, want %d: %s", method, path, response.StatusCode, want, data)
	}
	return data
}
func (f *auditWorkflowFixture) prepare(name string) (string, uuid.UUID) {
	f.t.Helper()
	f.session()
	host, err := f.store.Queries.GetHost(f.t.Context(), db.GetHostParams{ID: f.host, EnterpriseID: f.enterprise})
	if err != nil {
		f.t.Fatal(err)
	}
	raw := f.call("POST", "/enterprise/hosts/"+f.host.String()+"/actions/preview-update", map[string]any{"name": name, "expected_version": host.ResourceVersion}, 201, uuid.NewString())
	var pending struct {
		Ref string `json:"action_ref"`
	}
	if json.Unmarshal(raw, &pending) != nil || pending.Ref == "" {
		f.t.Fatal("missing action reference")
	}
	key := uuid.NewString()
	raw = f.call("POST", "/enterprise/pending-actions/"+pending.Ref+"/confirm", nil, 200, key)
	repeated := f.call("POST", "/enterprise/pending-actions/"+pending.Ref+"/confirm", nil, 200, key)
	if !bytes.Equal(raw, repeated) {
		f.t.Fatal("confirmation replay changed execution")
	}
	var result struct {
		Execution struct {
			ID uuid.UUID `json:"execution_id"`
		} `json:"execution"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || result.Execution.ID == uuid.Nil {
		f.t.Fatal("missing execution", err)
	}
	return pending.Ref, result.Execution.ID
}
func (f *auditWorkflowFixture) run(execution uuid.UUID) error {
	f.t.Helper()
	payload, _ := json.Marshal(actionservice.ExecutionTask{ExecutionID: execution, EnterpriseID: f.enterprise})
	return f.executor.Handle(f.t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}})
}

// Stop workers before the owned database is dropped.
func (f *auditWorkflowFixture) startWorker() {
	ctx, cancel := context.WithCancel(f.t.Context())
	done := make(chan error, 1)
	go func() {
		done <- (runtime.Processor{Store: f.store, Queue: "action", Owner: "audit-browser", Poll: 50 * time.Millisecond, Handle: f.executor}).Run(ctx)
	}()
	f.t.Cleanup(func() { cancel(); <-done })
}
