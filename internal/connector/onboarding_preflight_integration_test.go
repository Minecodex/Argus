package connector

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/installinstruction"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// The caller supplies a disposable database and owns its cleanup. This test
// exercises the real connection-test and Preview/Commit validation boundaries.
func TestOnboardingPreflightPostgres(t *testing.T) {
	databaseURL := os.Getenv("ARGUS_ONBOARDING_PREFLIGHT_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ARGUS_ONBOARDING_PREFLIGHT_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	root, _ := filepath.Abs("../..")
	if err := postgres.RunMigrations(ctx, databaseURL, filepath.Join(root, "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	enterpriseID, actorID, departmentID, credentialID, secretID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bastionPreviewSQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Callback preflight',$2,'UTC')", enterpriseID, "callback-"+enterpriseID.String())
	bastionPreviewSQL(t, store, "INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Callback tests')", departmentID, enterpriseID)
	bastionPreviewSQL(t, store, "INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'callback-test-user','Callback test user')", actorID, enterpriseID, departmentID)
	bastionPreviewSQL(t, store, "INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'SSH callback','ssh_password',$3)", secretID, enterpriseID, actorID)
	bastionPreviewSQL(t, store, "INSERT INTO credentials(id,enterprise_id,name,protocol,username,secret_id) VALUES($1,$2,'SSH callback','ssh','root',$3)", credentialID, enterpriseID, secretID)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}, Key: bytes.Repeat([]byte{4}, 32)}
	bastions := BastionService{Store: store, Actions: actions, Enrollment: Service{EnrollmentURL: "https://argus.test", GatewayEndpoint: "grpcs://gateway.test:9443", TrustConfig: installinstruction.TrustConfig{TrustBundlePath: preflightTestCA(t), TrustBundleEpoch: 1}}}
	hosts := resource.Service{Store: store, Actions: actions, HostOnboarding: resourceNameOnboardingStub{}, OnboardingProbes: bastions}
	subject := resource.Subject{ActorID: actorID.String(), AuthorizationVersion: 1}
	input := resource.HostInput{Name: "Callback host", Address: "192.0.2.40", Port: 22, Platform: "linux", Role: "managed_host", SSHPath: "direct_executor", InstallMethod: "ssh", ControlPath: "direct", CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}, Username: "root", Environment: "development"}
	makeTest := func(controlPath string, verified bool) db.ConnectionTest {
		t.Helper()
		request := input
		request.OnboardingControlPath = controlPath
		test, err := hosts.CreateHostConnectionTest(ctx, subject, enterpriseID, request, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		result := resource.ConnectionTestResult{HostKeyFingerprint: "SHA256:callback-host", Platform: "linux", Architecture: "amd64", DistributionVersion: "debian:12", ServiceManager: "systemd", Privileged: true, FreeDiskBytes: 1 << 30, ResolvedIPs: []string{"192.0.2.40"}, CallbackVerified: verified}
		if verified {
			result.CallbackControlPath = controlPath
		}
		test, err = hosts.CompleteConnectionTest(ctx, enterpriseID, test.ID, test.RequestHash, "succeeded", result, "")
		if err != nil {
			t.Fatal(err)
		}
		return test
	}
	generic := makeTest("", false)
	if generic.Status != "succeeded" {
		t.Fatal("SSH-only removal probe failed")
	}
	input.ConnectionTestID = uuid.NullUUID{UUID: generic.ID, Valid: true}
	if _, err := hosts.PreviewCreateHost(ctx, subject, enterpriseID, input, uuid.NewString()); !errors.Is(err, resource.ErrConnectionTestNeeded) {
		t.Fatalf("generic SSH authorized Host Preview: %v", err)
	}
	bastionInput := BastionInput{Name: "Callback bastion", Address: input.Address, Port: input.Port, Username: input.Username, CredentialID: input.CredentialID, ConnectionTestID: input.ConnectionTestID, InstallMode: "direct_install"}
	if _, _, err := bastions.validateDirectInstallInput(ctx, store.Queries, enterpriseID, bastionInput); !errors.Is(err, resource.ErrConnectionTestNeeded) {
		t.Fatalf("generic SSH authorized Bastion: %v", err)
	}
	missing := makeTest("direct", false)
	if missing.Status != "failed" || missing.ErrorCode.String != "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID" {
		t.Fatalf("missing callback proof persisted as %s/%s", missing.Status, missing.ErrorCode.String)
	}
	valid := makeTest("direct", true)
	input.ConnectionTestID = uuid.NullUUID{UUID: valid.ID, Valid: true}
	bastionInput.ConnectionTestID = input.ConnectionTestID
	if _, _, err := bastions.validateDirectInstallInput(ctx, store.Queries, enterpriseID, bastionInput); err != nil {
		t.Fatal(err)
	}
	bastionInput.InstallMode = "direct_install_tunnel"
	if _, _, err := bastions.validateDirectInstallInput(ctx, store.Queries, enterpriseID, bastionInput); !errors.Is(err, resource.ErrConnectionTestNeeded) {
		t.Fatalf("direct proof authorized Bastion tunnel: %v", err)
	}
	action, err := hosts.PreviewCreateHost(ctx, subject, enterpriseID, input, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := store.Queries.GetPendingActionPlan(ctx, db.GetPendingActionPlanParams{ActionRef: action.ActionRef, EnterpriseID: enterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hosts.RevalidatePendingAction(ctx, store.Queries, action, frozen.ImmutablePlan); err != nil {
		t.Fatalf("matching callback rejected at Commit: %v", err)
	}
	bastions.Enrollment.TrustBundleEpoch++
	hosts.OnboardingProbes = bastions
	if _, err := hosts.RevalidatePendingAction(ctx, store.Queries, action, frozen.ImmutablePlan); !errors.Is(err, resource.ErrActionInvalidated) {
		t.Fatalf("CA epoch drift authorized Commit: %v", err)
	}
	bastionInput.InstallMode = "direct_install"
	if _, _, err := bastions.validateDirectInstallInput(ctx, store.Queries, enterpriseID, bastionInput); !errors.Is(err, resource.ErrConnectionTestNeeded) {
		t.Fatalf("CA epoch drift authorized Bastion: %v", err)
	}
	assertRelayPreflightDrift(t, store, bastions, enterpriseID)
}

func assertRelayPreflightDrift(t *testing.T, store *postgres.Store, service BastionService, enterpriseID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	scopeID := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	bastionPreviewSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status,relay_status,relay_address,relay_port_generation) VALUES($1,$2,'Callback relay','development',decode(repeat('00',32),'hex'),'command','active','ready','10.20.30.40',3)", scopeID.UUID, enterpriseID)
	plan, err := service.PlanHostOnboardingProbe(ctx, store.Queries, enterpriseID, "bastion_relay", "bastion_connector", scopeID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.EnrollDialAddress != "10.20.30.40:8445" || plan.GatewayDialAddress != "10.20.30.40:9445" || plan.RelayPortGeneration != 3 {
		t.Fatalf("relay callback did not use authoritative scope: %+v", plan)
	}
	raw, _ := json.Marshal(map[string]*installation.CallbackProbePlan{"onboarding": plan})
	result, _ := json.Marshal(resource.ConnectionTestResult{CallbackVerified: true, CallbackControlPath: "bastion_relay"})
	test := db.ConnectionTest{RequestPlan: raw, Result: result}
	if err := service.validateCallbackConnectionTest(ctx, store.Queries, enterpriseID, test, "bastion_relay", "bastion_connector", scopeID); err != nil {
		t.Fatal(err)
	}
	bastionPreviewSQL(t, store, "UPDATE bastion_scopes SET relay_port_generation=4,relay_https_port=8446 WHERE id=$1", scopeID.UUID)
	if err := service.validateCallbackConnectionTest(ctx, store.Queries, enterpriseID, test, "bastion_relay", "bastion_connector", scopeID); !errors.Is(err, resource.ErrConnectionTestNeeded) {
		t.Fatalf("relay drift authorized callback: %v", err)
	}
	for _, path := range []string{"direct", "executor_tunnel"} {
		plan, err := service.PlanHostOnboardingProbe(ctx, store.Queries, enterpriseID, path, "direct_executor", uuid.NullUUID{})
		if err != nil || plan.EnrollDialAddress != "" || plan.GatewayDialAddress != "" || plan.RelayPortGeneration != 0 {
			t.Fatalf("%s leaked dial overrides: %+v %v", path, plan, err)
		}
	}
}

func preflightTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Callback test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "callback-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
