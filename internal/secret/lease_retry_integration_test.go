package secret

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestIssueLeaseRetriesSerializationBeforeDelivery(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_SECRET_TEST_DATABASE_URL", "secret_lease", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	enterprise, department, user := uuid.New(), uuid.New(), uuid.New()
	secretID, credentialID, target := uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'lease test','UTC')", enterprise)
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'lease test')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'lease-user','lease user')", user, enterprise, department)
	exec("INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'lease secret','kubeconfig',$3)", secretID, enterprise, user)
	keyring := testKeyring(t, 1)
	envelope, err := keyring.Encrypt([]byte("private test credential"), secretAAD(enterprise, secretID, 1, "kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO secret_versions(id,secret_id,enterprise_id,version,provider,key_id,key_version,wrapped_dek,wrap_nonce,nonce,ciphertext,value_hash) VALUES($1,$2,$3,1,$4,$5,$6,$7,$8,$9,$10,$11)",
		uuid.New(), secretID, enterprise, envelope.Provider, envelope.KeyID, envelope.KeyVersion,
		envelope.WrappedDEK, envelope.WrapNonce, envelope.Nonce, envelope.Ciphertext, envelope.ValueHash)
	exec("INSERT INTO credentials(id,enterprise_id,name,protocol,secret_id) VALUES($1,$2,'lease credential','kubernetes',$3)", credentialID, enterprise, secretID)
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	blocker, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(ctx, "SELECT id FROM secrets WHERE id=$1 FOR UPDATE", secretID); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: store, Keyring: keyring}
	request := LeaseRequest{CredentialID: credentialID, OperationRef: "serialization-regression",
		TargetResourceType: "kubernetes_cluster", TargetResourceID: target, RecipientType: "direct_executor",
		RecipientID: "lease-regression", Protocol: "kubernetes", TTL: time.Minute}
	type outcome struct {
		lease IssuedLease
		err   error
	}
	finished := make(chan outcome, 1)
	go func() {
		lease, err := service.IssueLease(WithActorType(ctx, "direct_executor"), "lease-regression", enterprise, request)
		finished <- outcome{lease, err}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		err = store.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%UPDATE secrets SET last_accessed_at%')").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lease did not reach the controlled row conflict")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = blocker.Exec(ctx, "UPDATE secrets SET last_accessed_at=now() WHERE id=$1", secretID); err != nil {
		t.Fatal(err)
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var result outcome
	select {
	case result = <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("credential lease did not recover from serialization conflict")
	}
	defer clear(result.lease.Value)
	if result.err != nil || string(result.lease.Value) != "private test credential" {
		t.Fatalf("lease failed after retry: %v", result.err)
	}
	var leases, events int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM credential_leases WHERE credential_id=$1", credentialID).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE enterprise_id=$1 AND action='credential.use'", enterprise).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if leases != 1 || events != 1 {
		t.Fatalf("rolled-back attempt persisted state: leases=%d audit events=%d", leases, events)
	}
}
