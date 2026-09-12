package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestResourceNameAvailabilityPostgres(t *testing.T) {
	store := resourceNameTestStore(t)
	ctx := context.Background()
	enterpriseID, otherEnterpriseID := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{enterpriseID, otherEnterpriseID} {
		bastionPreviewSQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Resource names',$2,'UTC')", id, "names-"+id.String())
	}
	for _, item := range []struct{ name, status string }{{"Existing Host", "active"}, {"Disabled Host", "disabled"}, {"Uninstalled Host", "uninstalled"}, {"Deleted Host", "deleted"}} {
		bastionPreviewSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,status) VALUES($1,$2,$3,'',0,'linux','amd64','development',decode(repeat('00',32),'hex'),'managed_host','direct',$4)", uuid.New(), enterpriseID, item.name, item.status)
	}
	for _, item := range []struct{ name, status string }{{"Root Bastion", "active"}, {"Deleted Bastion", "deleted"}, {"Scope Only", "pending"}} {
		scopeID := uuid.New()
		bastionPreviewSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,$3,'development',decode(repeat('00',32),'hex'),'command',$4)", scopeID, enterpriseID, item.name, item.status)
		if item.name != "Scope Only" {
			bastionPreviewSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,address,port,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,status) VALUES($1,$2,$3,'',0,'linux','amd64','development',decode(repeat('00',32),'hex'),'bastion','direct',$4,$5)", uuid.New(), enterpriseID, item.name, scopeID, item.status)
		}
	}
	hostService := resource.Service{Store: store}
	bastionService := BastionService{Store: store}
	for _, item := range []struct {
		name                     string
		enterprise               uuid.UUID
		hostAvailable, available bool
	}{
		{" existing HOST ", enterpriseID, false, false},
		{"disabled host", enterpriseID, false, false},
		{"uninstalled host", enterpriseID, false, false},
		{"deleted host", enterpriseID, true, true},
		{"root bastion", enterpriseID, false, false},
		{"deleted bastion", enterpriseID, true, true},
		{"scope only", enterpriseID, true, false},
		{"new name", enterpriseID, true, true},
		{"Existing Host", otherEnterpriseID, true, true},
		{"Root Bastion", otherEnterpriseID, true, true},
	} {
		t.Run(item.name+"/"+item.enterprise.String(), func(t *testing.T) {
			available, err := hostService.HostNameAvailable(ctx, item.enterprise, item.name)
			if err != nil || available != item.hostAvailable {
				t.Fatalf("host availability = %t, error %v; want %t", available, err, item.hostAvailable)
			}
			available, err = bastionService.NameAvailable(ctx, item.enterprise, item.name)
			if err != nil || available != item.available {
				t.Fatalf("bastion availability = %t, error %v; want %t", available, err, item.available)
			}
		})
	}

	t.Run("host conflict precedes invalid SSH evidence", func(t *testing.T) {
		service := resource.Service{Store: store}
		preview, err := service.PreviewCreateHost(ctx, resource.Subject{}, enterpriseID, resource.HostInput{
			Name: " existing HOST ", Platform: "linux", Role: "managed_host", ControlPath: "direct", InstallMethod: "ssh", SSHPath: "direct_executor",
		}, "conflicting-host")
		if !errors.Is(err, resource.ErrResourceNameConflict) || preview.ID != uuid.Nil {
			t.Fatalf("conflicting preview = %s, error %v", preview.ID, err)
		}
	})
	t.Run("bastion conflict precedes invalid SSH evidence", func(t *testing.T) {
		preview, err := bastionService.PreviewCreate(ctx, resource.Subject{}, enterpriseID, BastionInput{Name: " EXISTING HOST ", InstallMode: "direct_install"}, "conflicting-bastion")
		if !errors.Is(err, ErrBastionNameConflict) || preview.ID != uuid.Nil {
			t.Fatalf("conflicting preview = %s, error %v", preview.ID, err)
		}
	})
	for _, table := range []string{"pending_actions", "pending_action_plans", "pending_action_tokens", "host_onboarding_operations", "connector_install_operations"} {
		var count int
		if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("conflicting name created %d rows in %s: %v", count, table, err)
		}
	}
	t.Run("creation freezes normalized names", func(t *testing.T) {
		assertResourceNamePreviewNormalization(t, store, enterpriseID)
	})
	t.Run("concurrent host creates retain final unique guard", func(t *testing.T) {
		assertConcurrentResourceNameGuard(t, enterpriseID, hostService.HostNameAvailable, "Host Race", "hosts_name_unique", func(name string) error {
			_, err := store.Queries.CreateHost(ctx, db.CreateHostParams{ID: uuid.New(), EnterpriseID: enterpriseID, Name: name, Platform: "linux", Architecture: pgtype.Text{String: "amd64", Valid: true},
				Role: "managed_host", ControlPath: "direct", Environment: "development", Labels: []byte(`{}`), LabelsHash: make([]byte, 32), ConnectionStatus: "onboarding"})
			return err
		})
	})
	t.Run("concurrent bastion creates retain final unique guard", func(t *testing.T) {
		assertConcurrentResourceNameGuard(t, enterpriseID, bastionService.NameAvailable, "Bastion Race", "bastion_scopes_name_unique", func(name string) error {
			_, err := store.Queries.CreateBastionScope(ctx, db.CreateBastionScopeParams{ID: uuid.New(), EnterpriseID: enterpriseID, Name: name,
				Environment: "development", Labels: []byte(`{}`), LabelsHash: make([]byte, 32), OnboardingMode: "command"})
			return err
		})
	})
}

func TestResourceNameValidationPrecedesDatabaseAccess(t *testing.T) {
	ctx := context.Background()
	enterpriseID := uuid.New()
	for _, name := range []string{"", " \t\u3000", strings.Repeat("主", 129)} {
		for _, check := range []func(context.Context, uuid.UUID, string) (bool, error){(resource.Service{}).HostNameAvailable, (BastionService{}).NameAvailable} {
			available, err := check(ctx, enterpriseID, name)
			if available || !errors.Is(err, resource.ErrInvalidResourceName) {
				t.Fatalf("invalid name availability = %t, error %v", available, err)
			}
		}
		if _, err := (resource.Service{}).PreviewCreateHost(ctx, resource.Subject{}, enterpriseID, resource.HostInput{Name: name}, "invalid-host"); !errors.Is(err, resource.ErrInvalidResourceName) {
			t.Fatalf("host preview invalid name error = %v", err)
		}
		if _, err := (BastionService{}).PreviewCreate(ctx, resource.Subject{}, enterpriseID, BastionInput{Name: name}, "invalid-bastion"); !errors.Is(err, resource.ErrInvalidResourceName) {
			t.Fatalf("bastion preview invalid name error = %v", err)
		}
	}
}

func assertConcurrentResourceNameGuard(t *testing.T, enterpriseID uuid.UUID, check func(context.Context, uuid.UUID, string) (bool, error), name, constraint string, create func(string) error) {
	t.Helper()
	ctx := context.Background()
	names := []string{name, strings.ToLower(name)}
	for _, candidate := range names {
		available, err := check(ctx, enterpriseID, candidate)
		if err != nil || !available {
			t.Fatalf("precheck = %t, error %v", available, err)
		}
	}
	start, results := make(chan struct{}), make(chan error, 2)
	for _, candidate := range names {
		go func() { <-start; results <- create(candidate) }()
	}
	close(start)
	successes, conflicts := 0, 0
	for range names {
		err := <-results
		var databaseError *pgconn.PgError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == constraint:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results = %d successes, %d conflicts", successes, conflicts)
	}
	available, err := check(ctx, enterpriseID, name)
	if err != nil || available {
		t.Fatalf("committed name availability = %t, error %v", available, err)
	}
}

func assertResourceNamePreviewNormalization(t *testing.T, store *postgres.Store, enterpriseID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}, Key: bytes.Repeat([]byte{4}, 32)}
	subject := resource.Subject{ActorID: uuid.NewString(), AuthorizationVersion: 1}
	departmentID := uuid.New()
	bastionPreviewSQL(t, store, "INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Name tests')", departmentID, enterpriseID)
	bastionPreviewSQL(t, store, "INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'name-test-user','Name test user')", subject.ActorID, enterpriseID, departmentID)
	hosts := resource.Service{Store: store, Actions: actions, HostOnboarding: resourceNameOnboardingStub{}}
	hostAction, err := hosts.PreviewCreateHost(ctx, subject, enterpriseID, resource.HostInput{Name: " \t Normalized Host \u3000", Platform: "linux", Architecture: "amd64", Role: "managed_host", ControlPath: "direct", InstallMethod: "manual", SSHPath: "none", Environment: "development"}, "normalized-host")
	if err != nil {
		t.Fatal(err)
	}
	hostPlan := assertFrozenResourceName(t, store, hostAction, "Normalized Host")
	if err := store.InTx(ctx, func(q *db.Queries) error {
		_, err := hosts.CommitPendingAction(ctx, q, hostAction, hostPlan.ImmutablePlan)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	host, err := store.Queries.GetHost(ctx, db.GetHostParams{ID: hostAction.ResourceID.UUID, EnterpriseID: enterpriseID})
	if err != nil || host.Name != "Normalized Host" {
		t.Fatalf("created host name = %q, error %v", host.Name, err)
	}

	manifest := connectorReleaseManifest{SchemaVersion: "argus.connector_release/v3", ManifestURI: "https://artifacts.invalid/manifest.json", Installers: testConnectorInstallers(), SigningKeyID: "release-key", SigningPublicKey: strings.Repeat("a", 43)}
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		manifest.Artifacts = append(manifest.Artifacts, connectorReleaseArtifact{Platform: platform, URI: "https://artifacts.invalid/" + platform, SHA256: strings.Repeat("a", 64), Signature: strings.Repeat("b", 86), SigningKeyID: "release-key", ByteSize: 100})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bastionPreviewSQL(t, store, "INSERT INTO connector_release_versions(id,version,manifest,manifest_hash) VALUES($1,'names-test',$2,decode(repeat('00',32),'hex'))", uuid.New(), raw)
	bastions := BastionService{Store: store, Actions: actions}
	bastionAction, err := bastions.PreviewCreate(ctx, subject, enterpriseID, BastionInput{Name: " \t Normalized Bastion \u3000", Architecture: "amd64", InstallMode: "command", Environment: "development"}, "normalized-bastion")
	if err != nil {
		t.Fatal(err)
	}
	assertFrozenResourceName(t, store, bastionAction, "Normalized Bastion")
}

func assertFrozenResourceName(t *testing.T, store *postgres.Store, action db.PendingAction, want string) db.PendingActionPlan {
	t.Helper()
	plan, err := store.Queries.GetPendingActionPlan(context.Background(), db.GetPendingActionPlanParams{ActionRef: action.ActionRef, EnterpriseID: action.EnterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	var frozen struct{ Input struct{ Name string } }
	var preview struct{ Name string }
	if err = json.Unmarshal(plan.ImmutablePlan, &frozen); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(action.Preview, &preview); err != nil {
		t.Fatal(err)
	}
	if frozen.Input.Name != want || preview.Name != want {
		t.Fatalf("frozen name / preview name = %q / %q; want %q", frozen.Input.Name, preview.Name, want)
	}
	return plan
}

// External Connector installation is outside name validation. Keep the real
// domain preview, frozen plan, host insert and final uniqueness constraint.
type resourceNameOnboardingStub struct{}

func (resourceNameOnboardingStub) PrepareHostConnectorOnboarding(context.Context, *db.Queries, uuid.UUID, uuid.UUID, resource.HostInput, string) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (resourceNameOnboardingStub) RevalidateHostConnectorOnboarding(context.Context, *db.Queries, uuid.UUID, json.RawMessage) error {
	return nil
}

func (resourceNameOnboardingStub) CommitHostConnectorOnboarding(context.Context, *db.Queries, db.PendingAction, db.Host, json.RawMessage) (resource.ActionCommitResult, error) {
	return resource.ActionCommitResult{}, nil
}

func resourceNameTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	if os.Getenv("ARGUS_RESOURCE_NAMES_INTEGRATION") != "1" {
		t.Skip("set ARGUS_RESOURCE_NAMES_INTEGRATION=1 to run the disposable PostgreSQL test")
	}
	ctx := context.Background()
	// CI may supply its own disposable PostgreSQL; the caller then owns cleanup.
	databaseURL := os.Getenv("ARGUS_RESOURCE_NAMES_DATABASE_URL")
	if databaseURL == "" {
		container := "argus-resource-names-pg-" + uuid.NewString()[:8]
		t.Cleanup(func() {
			output, err := resourceNameDocker("rm", "-f", "-v", container)
			if err != nil && !strings.Contains(output, "No such container") {
				t.Errorf("remove disposable PostgreSQL %s: %v: %s", container, err, output)
			}
		})
		if output, err := resourceNameDocker("run", "-d", "--name", container, "-e", "POSTGRES_PASSWORD=resource-names-test-only", "-p", "127.0.0.1::5432", "postgres:18.6-alpine"); err != nil {
			t.Fatalf("start disposable PostgreSQL: %v: %s", err, output)
		}
		output, err := resourceNameDocker("port", container, "5432/tcp")
		if err != nil {
			t.Fatalf("discover disposable PostgreSQL port: %v: %s", err, output)
		}
		databaseURL = "postgres://postgres:resource-names-test-only@" + strings.TrimSpace(output) + "/postgres?sslmode=disable"
	}
	var store *postgres.Store
	var err error
	for range 40 {
		store, err = postgres.Open(ctx, databaseURL)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err = postgres.RunMigrations(ctx, databaseURL, filepath.Join(root, "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	return store
}

func resourceNameDocker(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	return string(output), err
}
