package platform

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func registryBootstrapFixture(t *testing.T) (*postgres.Store, EnterpriseService) {
	t.Helper()
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "registry_bootstrap", filepath.Join("..", "..", "migrations", "postgresql"))
	if err := audit.InitializeChain(t.Context(), store.Queries, "platform", uuid.NullUUID{}); err != nil {
		t.Fatal(err)
	}
	return store, EnterpriseService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}}
}

func TestPostgresEnterpriseCreationAndRegistryStartupSerialize(t *testing.T) {
	store, service := registryBootstrapFixture(t)
	start := make(chan struct{})
	type created struct {
		enterprise db.Enterprise
		err        error
	}
	creation := make(chan created, 1)
	reconciliation := make(chan error, 1)
	go func() {
		<-start
		value, err := service.CreateEnterprise(t.Context(), uuid.NewString(), CreateEnterpriseInput{Name: "Owned concurrent bootstrap", Code: "registry-" + uuid.NewString(), Timezone: "UTC", DefaultLocale: "en-US"}, uuid.NewString())
		creation <- created{value, err}
	}()
	go func() {
		<-start
		_, err := (authorization.Service{Store: store}).ReconcileBuiltinRegistry(t.Context())
		reconciliation <- err
	}()
	close(start)
	result, reconcileErr := <-creation, <-reconciliation
	if result.err != nil || reconcileErr != nil {
		t.Fatalf("concurrent enterprise/registry bootstrap: %v / %v", result.err, reconcileErr)
	}
	roles, err := store.Queries.ListRoles(t.Context(), result.enterprise.ID)
	if err != nil || len(roles) != len(authorization.BuiltinRoles) {
		t.Fatal("enterprise has incomplete or duplicate built-in roles", err)
	}
	for _, role := range roles {
		actual, err := store.Queries.ListRolePermissions(t.Context(), role.ID)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(actual)
		found := false
		for _, definition := range authorization.BuiltinRoles {
			if role.IdentityKey.String != definition.Key {
				continue
			}
			found = true
			wanted := append([]string(nil), definition.Permissions...)
			slices.Sort(wanted)
			if !role.Builtin || !slices.Equal(actual, wanted) {
				t.Fatal("concurrent bootstrap left stale role permissions")
			}
		}
		if !found {
			t.Fatal("unexpected managed role")
		}
	}
}

func TestPostgresEnterpriseCreationRejectsNewerRegistry(t *testing.T) {
	store, service := registryBootstrapFixture(t)
	if _, err := store.Pool.Exec(t.Context(), "INSERT INTO permissions(id,description,registry_version) VALUES('owned.future','Future registry',$1)", authorization.PermissionRegistryVersion+1); err != nil {
		t.Fatal(err)
	}
	_, err := service.CreateEnterprise(t.Context(), uuid.NewString(), CreateEnterpriseInput{Name: "Owned stale bootstrap", Code: "stale-" + uuid.NewString(), Timezone: "UTC", DefaultLocale: "en-US"}, uuid.NewString())
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatal("stale enterprise creation overwrote a newer registry", err)
	}
	var count int
	if err := store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM enterprises").Scan(&count); err != nil || count != 0 {
		t.Fatal("stale registry admission left a partial enterprise", err)
	}
}
