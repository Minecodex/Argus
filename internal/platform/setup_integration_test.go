package platform

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestPostgresSetupCreatesIndependentAdministratorsOnce(t *testing.T) {
	ctx := t.Context()
	store := postgrestest.New(t, "ARGUS_P5_TEST_DATABASE_URL", "first_setup", filepath.Join("..", "..", "migrations", "postgresql"))
	service := SetupService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}}
	input := SetupInput{
		EnterpriseName: "首家企业", Timezone: "Asia/Shanghai", ExternalURL: "https://argus.example.test",
		Username: "first-admin", DisplayName: "初始管理员", Password: "StrongSetup2026!",
	}

	// Reject the final user insert after the platform user and enterprise
	// have been written, so a failure exercises the full transaction.
	if _, err := store.Pool.Exec(ctx, "ALTER TABLE enterprise_users ADD CONSTRAINT setup_fixture_reject CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Initialize(ctx, input, uuid.NewString()); err == nil {
		t.Fatal("setup succeeded despite the rejected enterprise user")
	}
	state, settings, err := service.Status(ctx)
	if err != nil || state.State != "uninitialized" || settings != nil {
		t.Fatalf("failed setup left partial platform state: %v, settings present=%t, %v", state.State, settings != nil, err)
	}
	if _, err = store.Pool.Exec(ctx, "ALTER TABLE enterprise_users DROP CONSTRAINT setup_fixture_reject"); err != nil {
		t.Fatal(err)
	}

	type attempt struct {
		key    string
		result SetupResult
		err    error
	}
	start, attempts := make(chan struct{}), make(chan attempt, 2)
	for range 2 {
		go func() {
			<-start
			key := uuid.NewString()
			result, err := service.Initialize(ctx, input, key)
			attempts <- attempt{key: key, result: result, err: err}
		}()
	}
	close(start)
	winner, loser := <-attempts, <-attempts
	if winner.err != nil {
		winner, loser = loser, winner
	}
	if winner.err != nil || !errors.Is(loser.err, ErrAlreadyInitialized) {
		t.Fatalf("expected one successful setup and one conflict: %v, %v", winner.err, loser.err)
	}
	result := winner.result
	replayed, err := service.Initialize(ctx, input, winner.key)
	if err != nil || replayed != result {
		t.Fatalf("setup replay returned %v, %v", replayed, err)
	}
	tenants, err := store.Queries.ListAllEnterprises(ctx)
	if err != nil || len(tenants) != 1 || tenants[0].ID != result.EnterpriseID ||
		tenants[0].Name != input.EnterpriseName || tenants[0].Timezone != input.Timezone {
		t.Fatalf("expected one initialized enterprise: %+v, %v", tenants, err)
	}
	platformUser, err := store.Queries.GetPlatformUserByUsername(ctx, input.Username)
	if err != nil || platformUser.ID != result.PlatformUserID || platformUser.DisplayName != input.DisplayName {
		t.Fatalf("platform administrator was not initialized: %v", err)
	}
	enterpriseUser, err := store.Queries.GetEnterpriseUserByUsername(ctx, input.Username)
	if err != nil || enterpriseUser.ID != result.EnterpriseUserID || enterpriseUser.ID == platformUser.ID ||
		enterpriseUser.EnterpriseID != result.EnterpriseID || enterpriseUser.DisplayName != input.DisplayName || enterpriseUser.DepartmentStatus != "active" {
		t.Fatalf("enterprise administrator was not initialized: %v", err)
	}
	var bound bool
	if err := store.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM role_bindings b JOIN roles r ON r.id = b.role_id
		 WHERE b.enterprise_id = $1 AND b.subject_type = 'user' AND b.subject_id = $2
		 AND r.identity_key = 'enterprise_admin' AND b.status = 'active')`, result.EnterpriseID, result.EnterpriseUserID).Scan(&bound); err != nil || !bound {
		t.Fatalf("initial administrator has no enterprise role: %v", err)
	}

	platformKey := db.GetPasswordCredentialParams{Audience: "platform", SubjectID: result.PlatformUserID}
	enterpriseKey := db.GetPasswordCredentialParams{Audience: "enterprise", SubjectID: result.EnterpriseUserID}
	checkPassword := func(key db.GetPasswordCredentialParams, password string) db.PasswordCredential {
		t.Helper()
		credential, err := store.Queries.GetPasswordCredential(ctx, key)
		if err != nil || credential.Temporary {
			t.Fatalf("%s credential is missing or temporary: %v", key.Audience, err)
		}
		ok, err := identity.VerifyPassword(credential.EncodedHash, password)
		if err != nil || !ok {
			t.Fatalf("%s password did not verify: %v", key.Audience, err)
		}
		return credential
	}
	platformCredential := checkPassword(platformKey, input.Password)
	enterpriseCredential := checkPassword(enterpriseKey, input.Password)
	if platformCredential.EncodedHash == enterpriseCredential.EncodedHash {
		t.Fatal("the two credentials share a hash")
	}
	principal := identity.Principal{Session: db.Session{Audience: "enterprise", UserID: result.EnterpriseUserID}}
	const nextPassword = "NewEnterprisePassword2027!"
	if err = (identity.Service{Store: store}).ChangePassword(ctx, principal, input.Password, nextPassword, enterpriseCredential.Version); err != nil {
		t.Fatal(err)
	}
	checkPassword(enterpriseKey, nextPassword)
	checkPassword(platformKey, input.Password)
}
