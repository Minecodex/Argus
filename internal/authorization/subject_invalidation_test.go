package authorization

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestPostgresRoleChangesInvalidateOnlyAffectedSubjects(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "auth_subjects", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	exec := func(query string, values ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, values...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, otherDepartment := uuid.New(), uuid.New(), uuid.New()
	admin, member, other, account, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Authorization scope',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$3,'members'),($2,$3,'other')", department, otherDepartment, enterprise)
	for _, user := range []uuid.UUID{admin, member, other} {
		dept := otherDepartment
		if user == member {
			dept = department
		}
		exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Scope fixture')", user, enterprise, dept, user.String())
	}
	exec("INSERT INTO service_accounts(id,enterprise_id,name) VALUES($1,$2,'evaluator')", account, enterprise)
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'target reader')", role, enterprise)
	for id, description := range PermissionRegistry {
		exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, PermissionRegistryVersion)
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'monitoring.read')", role)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{4}, 32)}}
	version := func(kind string, id uuid.UUID) int64 {
		t.Helper()
		value, err := service.CurrentAuthorizationVersion(ctx, enterprise, id, kind)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	assert := func(memberVersion, accountVersion int64) {
		t.Helper()
		if version("user", admin) != 1 || version("user", other) != 1 || version("user", member) != memberVersion || version("service_account", account) != accountVersion {
			t.Fatal("authorization change invalidated an unrelated subject or missed an affected one")
		}
	}
	foreignEnterprise, foreignDepartment := uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Foreign scope',$2,'UTC')", foreignEnterprise, foreignEnterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Foreign')", foreignDepartment, foreignEnterprise)
	if _, err := service.CreateBinding(ctx, admin.String(), enterprise, BindingInput{SubjectType: "department", SubjectID: foreignDepartment, RoleID: role}, uuid.NewString()); err == nil {
		t.Fatal("a foreign department acquired a local binding")
	}
	assert(1, 1)
	bind, err := service.CreateBinding(ctx, admin.String(), enterprise, BindingInput{SubjectType: "service_account", SubjectID: account, RoleID: role}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	assert(1, 2)
	if _, err = service.CreateBinding(ctx, admin.String(), enterprise, BindingInput{SubjectType: "user", SubjectID: member, RoleID: role}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	assert(2, 2)
	if _, err = service.CreateBinding(ctx, admin.String(), enterprise, BindingInput{SubjectType: "department", SubjectID: department, RoleID: role}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	assert(3, 2)
	inactive := "disabled"
	if _, err = service.UpdateBinding(ctx, admin.String(), enterprise, bind.Binding.ID, BindingInput{ExpectedVersion: bind.Binding.Version, Status: &inactive}); err != nil {
		t.Fatal(err)
	}
	assert(3, 3)
	updated, err := service.UpdateRole(ctx, admin.String(), enterprise, role, RoleInput{ExpectedVersion: 1, Permissions: []string{"monitoring.read", "application.read"}})
	if err != nil {
		t.Fatal(err)
	}
	assert(4, 3) // Direct and department membership must bump the user only once.
	if _, err = service.UpdateRole(ctx, admin.String(), enterprise, role, RoleInput{ExpectedVersion: updated.Role.Version, Status: &inactive}); err != nil {
		t.Fatal(err)
	}
	assert(5, 3)
	user, err := store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: enterprise, ID: admin})
	if err != nil || user.AuthorizationVersion != 1 {
		t.Fatal("unrelated administrator's authenticated version was invalidated")
	}
}
