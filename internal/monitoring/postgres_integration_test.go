package monitoring

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type fixture struct {
	t        *testing.T
	s        Service
	a, other Actor
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL via ARGUS_MONITORING_TEST_DATABASE_URL")
	}
	if err := postgres.RunMigrations(t.Context(), url, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	e, d, u, other, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := &fixture{t: t, s: Service{Store: store, CursorKey: bytes.Repeat([]byte{7}, 32)}, a: Actor{EnterpriseID: e, SubjectID: u, SubjectType: "user", AuthorizationVersion: 1}}
	f.other = f.a
	f.other.SubjectID = other
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8',$2,'UTC')", e, e.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Plan8')", d, e)
	for _, id := range []uuid.UUID{u, other} {
		f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Plan8 operator')", id, e, d, id.String())
	}
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Plan8')", role, e)
	for key, description := range authorization.PermissionRegistry {
		f.exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", key, description, authorization.PermissionRegistryVersion)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions WHERE id IN ('application.read','application.manage','monitoring.read','monitoring.manage','collector.use','kubernetes.read','pending_action.confirm')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'department',$3,$4)", uuid.New(), e, d, role)
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", nullID(e)); err != nil {
		t.Fatal(err)
	}
	f.s.Actions = resource.PendingActionService{Store: store, Key: f.s.CursorKey, Idempotency: postgres.Idempotency{Key: f.s.CursorKey}}
	keyfile := filepath.Join(t.TempDir(), "keyring.json")
	data, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))}})
	if err = os.WriteFile(keyfile, data, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyfile)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Credentials = secret.Service{Store: store, Keyring: keyring, Idempotency: f.s.Actions.Idempotency}
	f.s.Runtime = transactionOnlyRuntime{}
	return f
}

// Domain transaction tests deliberately stop before the external transport.
// Real Collector deployment is a separate required acceptance gate.
type transactionOnlyRuntime struct{}

func (transactionOnlyRuntime) Plan(context.Context, *db.Queries, uuid.UUID, uuid.UUID) (json.RawMessage, error) {
	return json.RawMessage(`{"controlled_transaction_test":true}`), nil
}
func (transactionOnlyRuntime) Commit(context.Context, *db.Queries, db.PendingAction, json.RawMessage) (resource.ActionCommitResult, error) {
	return resource.ActionCommitResult{}, nil
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.s.Store.Pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) apply(m Mutation) (resource.ActionCommitResult, error) {
	return f.applyCredentials(m, nil)
}

func (f *fixture) applyCredentials(m Mutation, credentials json.RawMessage) (resource.ActionCommitResult, error) {
	ctx := f.t.Context()
	p, err := f.s.PreviewWithCredentials(ctx, f.a, m, credentials, uuid.NewString())
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	x := ActionExtension{Service: f.s}
	r := resource.Service{Store: f.s.Store, Actions: f.s.Actions, Extension: x}
	workflow := actionservice.Service{Store: f.s.Store, Idempotency: f.s.Actions.Idempotency, Resources: r}
	confirmation, err := workflow.Confirm(ctx, f.a.SubjectID.String(), uuid.NewString(), f.a.EnterpriseID, f.a.AuthorizationVersion, false, p.ActionRef, uuid.NewString())
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	ctx, err = r.PreparePendingAction(ctx, confirmation.PendingAction)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	var result resource.ActionCommitResult
	err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		var err error
		result, err = f.s.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, x.RevalidateAction, x.CommitAction)
		return err
	})
	if err == nil {
		user, e := f.s.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: f.a.EnterpriseID, ID: f.a.SubjectID})
		if e != nil {
			return result, e
		}
		f.a.AuthorizationVersion = user.AuthorizationVersion
	}
	return result, err
}

func (f *fixture) application(name, environment string) Application {
	f.t.Helper()
	r, err := f.apply(Mutation{Kind: "application", Operation: "create", Application: &ApplicationInput{Name: name, Environment: environment}})
	if err != nil {
		f.t.Fatal(err)
	}
	a, err := f.s.Application(f.t.Context(), f.a, r.ResourceID)
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func TestPostgresApplicationPreviewCommitIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	prod := f.application("Orders", "production")
	test := f.application("Orders", "testing")
	if prod.ID == test.ID || prod.Environment == test.Environment {
		t.Fatal("environments were merged")
	}
	if _, err := f.s.Application(ctx, f.other, prod.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("ungranted application readable: %v", err)
	}
	page, err := f.s.Applications(ctx, f.other, ListInput{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("application catalog leaked: %+v %v", page, err)
	}
	if _, err = f.s.Preview(ctx, f.a, Mutation{Kind: "application", Operation: "update", ID: prod.ID, ExpectedVersion: prod.Version, Application: &ApplicationInput{Name: "Orders", Environment: "testing"}}, uuid.NewString()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("environment changed: %v", err)
	}
	if _, err = f.s.Store.Pool.Exec(ctx, "UPDATE monitoring_applications SET environment='testing' WHERE id=$1", prod.ID); err == nil {
		t.Fatal("database allowed environment reassignment")
	}
	page, err = f.s.Applications(ctx, f.a, ListInput{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("pagination: %+v %v", page, err)
	}
	next, err := f.s.Applications(ctx, f.a, ListInput{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("second page: %+v %v", next, err)
	}
	if _, err = f.s.Applications(ctx, f.other, ListInput{Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor accepted for another principal")
	}
	if _, err = f.s.Applications(ctx, f.a, ListInput{Limit: 1, Search: "different", Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor accepted for another filter")
	}
	if _, err = f.apply(Mutation{Kind: "application", Operation: "update", ID: prod.ID, ExpectedVersion: prod.Version, Application: &ApplicationInput{Name: "Orders v2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Preview(ctx, f.a, Mutation{Kind: "application", Operation: "archive", ID: prod.ID, ExpectedVersion: prod.Version}, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
}

func TestPostgresServiceDiscoveryIdentityAndForeignKeys(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	a := f.application("Orders", "production")
	params := db.DiscoverMonitoringServiceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ApplicationID: a.ID, ServiceNamespace: "payments", ServiceName: "api", Runtime: "go"}
	first, err := f.s.Store.Queries.DiscoverMonitoringService(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	params.ID = uuid.New()
	again, err := f.s.Store.Queries.DiscoverMonitoringService(ctx, params)
	if err != nil || again.ID != first.ID {
		t.Fatalf("service discovery not idempotent: %v", err)
	}
	params.ID = uuid.New()
	params.ServiceNamespace = "orders"
	other, err := f.s.Store.Queries.DiscoverMonitoringService(ctx, params)
	if err != nil || other.ID == first.ID {
		t.Fatalf("service namespaces merged: %v", err)
	}
	page, err := f.s.Services(ctx, f.a, ListInput{ApplicationIDs: []uuid.UUID{}})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("explicit empty application scope widened: %v %v", page, err)
	}
	page, err = f.s.Services(ctx, f.a, ListInput{ApplicationIDs: []uuid.UUID{a.ID}, Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("service cursor: %v", err)
	}
	if _, err = f.s.Services(ctx, f.a, ListInput{Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("explicit application cursor became all-authorized")
	}
	if _, err = f.s.Store.Pool.Exec(ctx, "INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_name) VALUES($1,$2,$3,'bad')", uuid.New(), uuid.New(), a.ID); err == nil {
		t.Fatal("cross-enterprise service accepted")
	}
}

func TestPostgresNamespaceGrantDoesNotFollowRecreatedName(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cluster := uuid.New()
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID)
	ns, err := f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, NamespaceUid: "first-uid", Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, ns.ID)
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.a, "kubernetes_namespace", ns.ID); err != nil {
		t.Fatal(err)
	}
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.a, "kubernetes_cluster", cluster); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("namespace granted cluster access")
	}
	if err = f.s.Store.Queries.RetireReplacedKubernetesNamespace(ctx, db.RetireReplacedKubernetesNamespaceParams{EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: "orders", NamespaceUid: "second-uid"}); err != nil {
		t.Fatal(err)
	}
	replacement, err := f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, NamespaceUid: "second-uid", Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, f.s.Store.Queries, f.a, "kubernetes_namespace")
	if err != nil || slices.Contains(ids, replacement.ID) || slices.Contains(ids, ns.ID) {
		t.Fatalf("stale namespace grant: %v %v", ids, err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, cluster)
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.a, "kubernetes_namespace", replacement.ID); err != nil {
		t.Fatal("cluster grant did not include current namespace", err)
	}
	grants := authorization.Service{Store: f.s.Store}
	catalog, err := grants.GrantCatalogPage(ctx, f.a.EnterpriseID, f.a.SubjectID, "user", "kubernetes_namespace", "", uuid.Nil, 201)
	if err != nil || len(catalog) != 1 || catalog[0].ResourceID != replacement.ID || !catalog[0].Inherited || catalog[0].Direct {
		t.Fatalf("namespace catalog did not report cluster inheritance: %+v %v", catalog, err)
	}
	if err = grants.UpdateGrantBatch(ctx, f.a.SubjectID.String(), f.a.EnterpriseID, authorization.GrantBatchInput{SubjectType: "user", SubjectID: f.a.SubjectID, ResourceType: "kubernetes_namespace", ResourceIDs: []uuid.UUID{replacement.ID}, Remove: true, ExpectedVersion: 1}); !errors.Is(err, authorization.ErrInheritedGrant) {
		t.Fatalf("inherited namespace grant removed independently: %v", err)
	}
}

func TestPostgresMonitoringCredentialIsPrivate(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	draft, collection := uuid.New(), uuid.New()
	var id uuid.UUID
	var credential db.Credential
	err := f.s.Store.InTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = f.s.Credentials.StageMonitoringWithQueries(ctx, q, f.a.EnterpriseID, f.a.SubjectID, draft, []byte(`{"username":"monitor","password":"private-fixture-value"}`))
		if err != nil {
			return err
		}
		credential, err = f.s.Credentials.BindMonitoringWithQueries(ctx, q, f.a.EnterpriseID, draft, id, collection)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Credentials.Get(ctx, f.a.EnterpriseID, id); !errors.Is(err, secret.ErrMonitoringManagedSecret) {
		t.Fatalf("private secret public by-ID read: %v", err)
	}
	items, err := f.s.Credentials.List(ctx, f.a.EnterpriseID)
	if err != nil || len(items) != 0 {
		t.Fatalf("private secret in platform catalog: %v", err)
	}
	rows, err := f.s.Store.Queries.ListCredentials(ctx, f.a.EnterpriseID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("private credential in platform catalog: %v", err)
	}
	if err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		return f.s.Credentials.RetireMonitoringWithQueries(ctx, q, f.a.EnterpriseID, uuid.New(), credential.ID)
	}); !errors.Is(err, secret.ErrCredentialUnavailable) {
		t.Fatalf("wrong owner retired credential: %v", err)
	}
	if err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		return f.s.Credentials.RetireMonitoringWithQueries(ctx, q, f.a.EnterpriseID, collection, credential.ID)
	}); err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetCredential(context.Background(), db.GetCredentialParams{EnterpriseID: f.a.EnterpriseID, ID: credential.ID})
	if err != nil || row.Status != "disabled" {
		t.Fatalf("credential retirement: %v", err)
	}
}
