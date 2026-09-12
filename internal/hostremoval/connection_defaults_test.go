package hostremoval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

type defaultsFixture struct {
	*db.Queries
	t               *testing.T
	enterprise      uuid.UUID
	host            db.Host
	scope           db.GetBastionScopeRow
	origin          db.HostOnboardingOperation
	bastion         db.ConnectorInstallOperation
	bastionOrigins  []*db.ConnectorInstallOperation
	credential      db.Credential
	secret          db.GetSecretRow
	hostError       error
	scopeError      error
	credentialError error
	reads           int
}

func (f *defaultsFixture) check(enterprise uuid.UUID) {
	f.t.Helper()
	f.reads++
	if enterprise != f.enterprise {
		f.t.Fatal("cross-enterprise lookup")
	}
}
func (f *defaultsFixture) GetHost(_ context.Context, p db.GetHostParams) (db.Host, error) {
	f.check(p.EnterpriseID)
	return f.host, f.hostError
}
func (f *defaultsFixture) GetBastionScope(_ context.Context, p db.GetBastionScopeParams) (db.GetBastionScopeRow, error) {
	f.check(p.EnterpriseID)
	return f.scope, f.scopeError
}
func (f *defaultsFixture) GetLatestSuccessfulHostOnboardingOperation(_ context.Context, p db.GetLatestSuccessfulHostOnboardingOperationParams) (db.HostOnboardingOperation, error) {
	f.check(p.EnterpriseID)
	if p.HostID != f.host.ID {
		f.t.Fatal("wrong Host")
	}
	return f.origin, nil
}
func (f *defaultsFixture) GetLatestHostOnboardingOperationByConnector(_ context.Context, p db.GetLatestHostOnboardingOperationByConnectorParams) (db.HostOnboardingOperation, error) {
	f.check(p.EnterpriseID)
	if p.HostID != f.host.ID || p.ConnectorID != f.host.ConnectorID.UUID {
		f.t.Fatal("wrong Host Connector identity")
	}
	return f.origin, nil
}
func (f *defaultsFixture) GetLatestConnectorInstallOperation(_ context.Context, p db.GetLatestConnectorInstallOperationParams) (db.ConnectorInstallOperation, error) {
	f.check(p.EnterpriseID)
	for _, operation := range f.bastionOrigins {
		if operation.ConnectorID == p.ConnectorID {
			return *operation, nil
		}
	}
	return db.ConnectorInstallOperation{}, pgx.ErrNoRows
}
func (f *defaultsFixture) GetCredential(_ context.Context, p db.GetCredentialParams) (db.Credential, error) {
	f.check(p.EnterpriseID)
	if p.ID != f.credential.ID {
		f.t.Fatal("wrong credential")
	}
	return f.credential, f.credentialError
}
func (f *defaultsFixture) GetSecret(_ context.Context, p db.GetSecretParams) (db.GetSecretRow, error) {
	f.check(p.EnterpriseID)
	if p.ID != f.credential.SecretID {
		f.t.Fatal("wrong Secret")
	}
	return f.secret, nil
}

func newDefaultsFixture(t *testing.T, bastion bool) (*defaultsFixture, resource.Subject, PreviewInput) {
	f := &defaultsFixture{t: t, enterprise: uuid.New()}
	f.host = db.Host{ID: uuid.New(), Role: TargetManagedHost, ResourceVersion: 3, ConnectorID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, Status: "active"}
	f.scope = db.GetBastionScopeRow{ID: uuid.New(), ResourceVersion: 4, ConnectorHostID: uuid.NullUUID{UUID: f.host.ID, Valid: true}, ActiveConnectorID: f.host.ConnectorID, OnboardingMode: "direct_install", Status: "active"}
	f.credential = db.Credential{ID: uuid.New(), SecretID: uuid.New(), Name: "installation-key", Username: "different-credential-account", Protocol: "ssh", Status: "active", Version: 9}
	f.secret = db.GetSecretRow{Status: "active"}
	hostPlan, _ := json.Marshal(installation.HostConnectorInstallPlan{HostID: f.host.ID, ConnectorID: f.host.ConnectorID.UUID,
		SSHPath: "direct_executor", Username: "root", CredentialID: f.credential.ID, CredentialVersion: 1, TrustBundlePEM: []byte("must-not-leak")})
	bastionPlan, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&connectorv1.ConnectorInstallCommand{
		HostId: f.host.ID.String(), ConnectorId: f.host.ConnectorID.UUID.String(), BastionScopeId: f.scope.ID.String(),
		TargetUsername: "administrator", CredentialId: f.credential.ID.String(), CredentialVersion: 1,
	})
	f.origin = db.HostOnboardingOperation{HostID: f.host.ID, ConnectorID: f.host.ConnectorID.UUID, InstallMethod: "ssh", SshPath: "direct_executor", Status: "succeeded", Plan: hostPlan}
	f.bastion = db.ConnectorInstallOperation{HostID: f.host.ID, ConnectorID: f.host.ConnectorID.UUID, BastionScopeID: f.scope.ID, Status: "succeeded", Plan: bastionPlan}
	f.bastionOrigins = []*db.ConnectorInstallOperation{&f.bastion}
	input := PreviewInput{TargetType: TargetManagedHost, TargetID: f.host.ID, ExpectedVersion: 3}
	if bastion {
		input = PreviewInput{TargetType: TargetBastion, TargetID: f.scope.ID, ExpectedVersion: 4}
	}
	return f, resource.Subject{AuthorizedResourceIDs: []uuid.UUID{f.host.ID}}, input
}

func TestRemovalConnectionDefaults(t *testing.T) {
	for _, kind := range []string{"direct_executor", "bastion_connector", "bastion"} {
		t.Run(kind, func(t *testing.T) {
			f, subject, input := newDefaultsFixture(t, kind == "bastion")
			f.origin.SshPath = kind
			got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
			if err != nil || got.Status != "available" || got.CredentialID == nil || *got.CredentialID != f.credential.ID {
				t.Fatalf("defaults: %+v %v", got, err)
			}
			want := "root"
			if kind == "bastion" {
				want = "administrator"
			}
			if got.Username != want {
				t.Fatalf("installation username not retained: %q", got.Username)
			}
			encoded, _ := json.Marshal(got)
			for _, secret := range []string{"must-not-leak", "password", "trust_bundle", "credential_version", "connection_test"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("unexpected response field %s", secret)
				}
			}
		})
	}
}

func TestBastionRemovalConnectionDefaultsIgnoreLaterFailedReplacement(t *testing.T) {
	f, subject, input := newDefaultsFixture(t, true)
	failedReplacementConnectorID := uuid.New()
	failed := f.bastion
	failed.ID, failed.ConnectorID, failed.Status = uuid.New(), failedReplacementConnectorID, "failed"
	f.bastionOrigins = []*db.ConnectorInstallOperation{&failed, &f.bastion}
	got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
	if err != nil || got.Status != "available" || got.CredentialID == nil || *got.CredentialID != f.credential.ID {
		t.Fatalf("failed replacement hid successful install defaults: %+v %v", got, err)
	}
}

func TestBastionRemovalConnectionDefaultsFollowCurrentConnectorProvenance(t *testing.T) {
	t.Run("command scope replaced over SSH", func(t *testing.T) {
		f, subject, input := newDefaultsFixture(t, true)
		f.scope.OnboardingMode = "command"
		got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if err != nil || got.Status != "available" || got.Username != "administrator" {
			t.Fatalf("current SSH replacement provenance ignored: %+v %v", got, err)
		}
	})
	t.Run("current command identity", func(t *testing.T) {
		f, subject, input := newDefaultsFixture(t, true)
		f.scope.OnboardingMode = "command"
		f.bastionOrigins = nil
		got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if err != nil || got.Status != "not_applicable" {
			t.Fatalf("command provenance = %+v %v", got, err)
		}
	})
}

func TestRemovalConnectionDefaultsRejectMismatchedFrozenInstallPlan(t *testing.T) {
	for _, bastion := range []bool{false, true} {
		f, subject, input := newDefaultsFixture(t, bastion)
		if bastion {
			var plan connectorv1.ConnectorInstallCommand
			if err := protojson.Unmarshal(f.bastion.Plan, &plan); err != nil {
				t.Fatal(err)
			}
			plan.BastionScopeId = uuid.NewString()
			f.bastion.Plan, _ = protojson.MarshalOptions{UseProtoNames: true}.Marshal(&plan)
		} else {
			var plan installation.HostConnectorInstallPlan
			if err := json.Unmarshal(f.origin.Plan, &plan); err != nil {
				t.Fatal(err)
			}
			plan.ConnectorID = uuid.New()
			f.origin.Plan, _ = json.Marshal(plan)
		}
		_, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if !errors.Is(err, ErrIdentityChanged) {
			t.Fatalf("mismatched frozen plan accepted: %v", err)
		}
	}
}

func TestRemovalConnectionDefaultsAcceptCurrentIdentityAfterWaitingOnlineTimeout(t *testing.T) {
	for _, bastion := range []bool{false, true} {
		f, subject, input := newDefaultsFixture(t, bastion)
		f.origin.Status = "failed"
		f.bastion.Status = "failed"
		got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if err != nil || got.Status != "available" {
			t.Fatalf("current enrolled identity lost install provenance: %+v %v", got, err)
		}
	}
}

func TestRemovalConnectionDefaultsReportUninstalledIdentity(t *testing.T) {
	for _, bastion := range []bool{false, true} {
		f, subject, input := newDefaultsFixture(t, bastion)
		if bastion {
			f.scope.ActiveConnectorID = uuid.NullUUID{}
		} else {
			f.host.ConnectorID = uuid.NullUUID{}
		}
		_, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("missing current Connector = %v, want ErrNotInstalled", err)
		}
	}
}

func TestRemovalConnectionDefaultsRejectStaleIdentityAndUnauthorizedTargets(t *testing.T) {
	for _, bastion := range []bool{false, true} {
		f, subject, input := newDefaultsFixture(t, bastion)
		_, err := readConnectionDefaults(context.Background(), f, resource.Subject{}, f.enterprise, input)
		wantReads := 0
		if bastion {
			wantReads = 1
		}
		if !errors.Is(err, ErrInvalidTarget) || f.reads != wantReads {
			t.Fatal("unauthorized resource was read")
		}
		wrong := input
		wrong.ExpectedVersion++
		_, err = readConnectionDefaults(context.Background(), f, subject, f.enterprise, wrong)
		if !errors.Is(err, resource.ErrVersionConflict) {
			t.Fatal("stale resource version accepted")
		}
		if bastion {
			f.bastion.HostID = uuid.New()
		} else {
			var plan installation.HostConnectorInstallPlan
			_ = json.Unmarshal(f.origin.Plan, &plan)
			plan.ConnectorID = uuid.New()
			f.origin.Plan, _ = json.Marshal(plan)
		}
		_, err = readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if !errors.Is(err, ErrIdentityChanged) {
			t.Fatalf("replaced identity accepted: %v", err)
		}
	}
}

func TestRemovalConnectionDefaultsUnavailableCredentials(t *testing.T) {
	for _, scenario := range []string{"deleted", "disabled", "wrong_protocol", "disabled_secret", "database_failure"} {
		t.Run(scenario, func(t *testing.T) {
			f, subject, input := newDefaultsFixture(t, false)
			switch scenario {
			case "deleted":
				f.credentialError = pgx.ErrNoRows
			case "disabled":
				f.credential.Status = "disabled"
			case "wrong_protocol":
				f.credential.Protocol = "kubernetes"
			case "disabled_secret":
				f.secret.Status = "disabled"
			case "database_failure":
				f.credentialError = errors.New("database unavailable")
			}
			got, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
			if scenario == "database_failure" {
				if err == nil {
					t.Fatal("database failure concealed")
				}
				return
			}
			if err != nil || got.Status != "credential_unavailable" || got.Username != "root" || got.CredentialID != nil || got.CredentialName != nil {
				t.Fatalf("unavailable defaults: %+v %v", got, err)
			}
		})
	}
}

func TestRemovalConnectionDefaultsPropagateTargetLookupFailures(t *testing.T) {
	databaseFailure := errors.New("database unavailable")
	for _, bastion := range []bool{false, true} {
		f, subject, input := newDefaultsFixture(t, bastion)
		if bastion {
			f.scopeError = databaseFailure
		} else {
			f.hostError = databaseFailure
		}
		_, err := readConnectionDefaults(context.Background(), f, subject, f.enterprise, input)
		if !errors.Is(err, databaseFailure) {
			t.Fatalf("target lookup failure became %v", err)
		}
	}
}
