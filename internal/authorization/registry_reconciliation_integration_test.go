package authorization

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

type ownedRegistryFixture struct {
	t                                                      *testing.T
	store                                                  *postgres.Store
	service                                                Service
	enterprise, admin, custom, user, other, account, grant uuid.UUID
	grantBefore                                            string
}

func newOwnedRegistryFixture(t *testing.T) *ownedRegistryFixture {
	t.Helper()
	full := filepath.Join("..", "..", "migrations", "postgresql")
	prior := t.TempDir()
	files, e := os.ReadDir(full)
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		var version int
		if _, e = fmt.Sscanf(file.Name(), "%d_", &version); e != nil || version > 19 {
			continue
		}
		body, e := os.ReadFile(filepath.Join(full, file.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(prior, file.Name()), body, 0600); e != nil {
			t.Fatal(e)
		}
	}
	f := &ownedRegistryFixture{t: t, store: postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "p8_registry_lifecycle", prior), enterprise: uuid.New(), admin: uuid.New(), custom: uuid.New(), user: uuid.New(), other: uuid.New(), account: uuid.New(), grant: uuid.New()}
	f.service = Service{Store: f.store}
	department, otherDepartment := uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned old enterprise',$2,'UTC')", f.enterprise, f.enterprise.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$3,'managed'),($2,$3,'custom')", department, otherDepartment, f.enterprise)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$5,$3,$1::uuid::text,'managed'),($2,$5,$4,$2::uuid::text,'custom')", f.user, f.other, department, otherDepartment, f.enterprise)
	f.exec("INSERT INTO service_accounts(id,enterprise_id,name) VALUES($1,$2,'owned worker')", f.account, f.enterprise)
	f.exec("INSERT INTO permissions(id,description,registry_version) VALUES('host.read','Old host read',14) ON CONFLICT DO NOTHING")
	f.exec("INSERT INTO roles(id,enterprise_id,identity_key,name,builtin) VALUES($1,$3,'enterprise_admin','Enterprise Admin',true),($2,$3,NULL,'Owned custom reader',false)", f.admin, f.custom, f.enterprise)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'host.read'),($2,'host.read')", f.admin, f.custom)
	for _, binding := range []struct {
		kind          string
		subject, role uuid.UUID
	}{{"user", f.user, f.admin}, {"department", department, f.admin}, {"service_account", f.account, f.admin}, {"user", f.other, f.custom}} {
		f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,$3,$4,$5)", uuid.New(), f.enterprise, binding.kind, binding.subject, binding.role)
	}
	host := uuid.New()
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,labels_hash,role,control_path) VALUES($1,$2,'Owned permission fixture','127.0.0.1',22,'development','linux',decode(repeat('00',32),'hex'),'managed_host','direct')", host, f.enterprise)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", f.grant, f.enterprise, f.user, host)
	if e = f.store.Pool.QueryRow(t.Context(), "SELECT to_jsonb(g)::text FROM data_authorization_grants g WHERE id=$1", f.grant).Scan(&f.grantBefore); e != nil {
		t.Fatal(e)
	}
	if e = postgres.RunMigrations(t.Context(), f.store.Pool.Config().ConnString(), full, postgres.MigrationUp); e != nil {
		t.Fatal(e)
	}
	if e = audit.InitializeChain(t.Context(), f.store.Queries, "enterprise", uuid.NullUUID{UUID: f.enterprise, Valid: true}); e != nil {
		t.Fatal(e)
	}
	return f
}
func (f *ownedRegistryFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, e := f.store.Pool.Exec(f.t.Context(), sql, args...); e != nil {
		f.t.Fatal(e)
	}
}
func (f *ownedRegistryFixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if e := f.store.Pool.QueryRow(f.t.Context(), sql, args...).Scan(&n); e != nil {
		f.t.Fatal(e)
	}
	return n
}
func (f *ownedRegistryFixture) assertPreserved() {
	f.t.Helper()
	var after string
	if e := f.store.Pool.QueryRow(f.t.Context(), "SELECT to_jsonb(g)::text FROM data_authorization_grants g WHERE id=$1", f.grant).Scan(&after); e != nil || after != f.grantBefore {
		f.t.Fatal("resource grant changed", e)
	}
	if f.count("SELECT count(*) FROM data_authorization_grants") != 1 {
		f.t.Fatal("registry implicitly granted resources")
	}
	if f.count("SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_id='host.read'", f.custom) != 1 || f.count("SELECT count(*) FROM role_permissions WHERE role_id=$1", f.custom) != 1 {
		f.t.Fatal("custom role changed")
	}
	if f.count("SELECT version FROM roles WHERE id=$1", f.custom) != 1 || f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.other) != 1 {
		f.t.Fatal("unrelated role/user version changed")
	}
}

func TestOwnedRegistryUpgradeIsAtomicIdempotentAndScoped(t *testing.T) {
	f := newOwnedRegistryFixture(t)
	result, e := f.service.ReconcileBuiltinRegistry(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if result.UpdatedRoles != 1 || result.CreatedRoles != len(BuiltinRoles)-1 {
		t.Fatal("unexpected managed role reconciliation", result)
	}
	for id := range PermissionRegistry {
		if f.count("SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_id=$2", f.admin, id) != 1 {
			t.Fatal("old administrator missing current permission", id)
		}
	}
	if f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.user) != 2 || f.count("SELECT authorization_version FROM service_accounts WHERE id=$1", f.account) != 2 {
		t.Fatal("bound subjects not invalidated exactly once")
	}
	if f.count("SELECT version FROM roles WHERE id=$1", f.admin) != 2 {
		t.Fatal("managed role version not advanced")
	}
	if _, e = f.service.UpdateRole(t.Context(), f.user.String(), f.enterprise, f.admin, RoleInput{ExpectedVersion: 2, Permissions: []string{"host.read"}}); !errors.Is(e, ErrBuiltinRoleImmutable) {
		t.Fatal("public role mutation can now edit a built-in role", e)
	}
	var raw []byte
	if e = f.store.Pool.QueryRow(t.Context(), "SELECT details FROM audit_events WHERE action='role.registry.upgrade'").Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var details map[string]any
	if json.Unmarshal(raw, &details) != nil || details["before"] == nil || details["after"] == nil {
		t.Fatal("registry audit lost before/after grants")
	}
	if f.count("SELECT count(*) FROM outbox_events WHERE topic='authorization.changed'") != 1 {
		t.Fatal("authorization invalidation outbox missing")
	}
	f.assertPreserved()
	again, e := f.service.ReconcileBuiltinRegistry(t.Context())
	if e != nil || again.CreatedRoles != 0 || again.UpdatedRoles != 0 {
		t.Fatal("registry reconciliation not idempotent", again, e)
	}
	if f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.user) != 2 || f.count("SELECT count(*) FROM outbox_events WHERE topic='authorization.changed'") != 1 {
		t.Fatal("idempotent retry repeated invalidation")
	}
	f.assertPreserved()
}
func TestOwnedRegistryConcurrentStartupReconcilesOnce(t *testing.T) {
	f := newOwnedRegistryFixture(t)
	type outcome struct {
		result RegistryReconciliation
		err    error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; r, e := f.service.ReconcileBuiltinRegistry(t.Context()); done <- outcome{r, e} }()
	}
	close(start)
	updates, created := 0, 0
	for i := 0; i < 2; i++ {
		o := <-done
		if o.err != nil {
			t.Fatal(o.err)
		}
		updates += o.result.UpdatedRoles
		created += o.result.CreatedRoles
	}
	if updates != 1 || created != len(BuiltinRoles)-1 || f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.user) != 2 {
		t.Fatal("concurrent startup repeated reconciliation")
	}
	f.assertPreserved()
}
func TestOwnedRegistryAuditFailureRollsBackAllChanges(t *testing.T) {
	f := newOwnedRegistryFixture(t)
	f.exec(`CREATE FUNCTION owned_registry_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'role.registry.%' THEN RAISE EXCEPTION 'owned audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER owned_registry_audit_failure BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION owned_registry_audit_failure()`)
	if _, e := f.service.ReconcileBuiltinRegistry(t.Context()); e == nil || !strings.Contains(e.Error(), "owned audit failure") {
		t.Fatal("expected exact injected audit failure", e)
	}
	if f.count("SELECT count(*) FROM permissions WHERE id='monitoring.manage'") != 0 || f.count("SELECT count(*) FROM roles") != 2 || f.count("SELECT version FROM roles WHERE id=$1", f.admin) != 1 || f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.user) != 1 || f.count("SELECT count(*) FROM outbox_events") != 0 {
		t.Fatal("failed reconciliation committed partial state")
	}
	f.assertPreserved()
}
func TestOwnedRegistryRejectsNewerRegistryWithoutMutation(t *testing.T) {
	f := newOwnedRegistryFixture(t)
	f.exec("INSERT INTO permissions(id,description,registry_version) VALUES('owned.future','Future registry', $1)", PermissionRegistryVersion+1)
	if _, e := f.service.ReconcileBuiltinRegistry(t.Context()); e == nil || !strings.Contains(e.Error(), "newer") {
		t.Fatal("newer registry was not fenced", e)
	}
	if f.count("SELECT count(*) FROM permissions WHERE id='monitoring.manage'") != 0 || f.count("SELECT version FROM roles WHERE id=$1", f.admin) != 1 {
		t.Fatal("stale service changed registry")
	}
	f.assertPreserved()
}

func TestOwnedRegistryMigrationCommandRepairsExistingRoles(t *testing.T) {
	f := newOwnedRegistryFixture(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "./cmd/argus-migrate", "up")
	command.Dir = root
	command.Env = append(os.Environ(), "ARGUS_DATABASE_URL="+f.store.Pool.Config().ConnString(), "ARGUS_MIGRATIONS_DIR="+filepath.Join(root, "migrations", "postgresql"))
	if _, err := command.CombinedOutput(); err != nil {
		t.Fatal("actual migration command did not reconcile the existing registry", err)
	}
	if f.count("SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_id='monitoring.manage'", f.admin) != 1 || f.count("SELECT authorization_version FROM enterprise_users WHERE id=$1", f.user) != 2 {
		t.Fatal("migration command omitted the shared registry lifecycle")
	}
	f.assertPreserved()
}
