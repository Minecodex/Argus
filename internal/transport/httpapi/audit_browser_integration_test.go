package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/installinstruction"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// The domain mutation, private-token consumption, encryption, HTTP handlers and
// browser are real. Only the installation payload is a harmless controlled command;
// this acceptance does not install software on a real host.
type auditCommandFixture struct{ resource.Service }

func (f auditCommandFixture) ExecutePendingAction(ctx context.Context, q *db.Queries, action db.PendingAction) (resource.ActionCommitResult, error) {
	result, err := f.Service.ExecutePendingAction(ctx, q, action)
	if err != nil {
		return result, err
	}
	expiry := time.Now().UTC().Add(5 * time.Minute)
	result.OneTimeCommand = &resource.OneTimeCommandResult{ExpiresAt: expiry, InstructionSets: []installinstruction.Set{{Scope: installinstruction.ScopeLinuxSystem, Platform: "linux_amd64", Shell: "posix_sh", Privilege: "system", Command: "echo audit-acceptance-claim-canary", BootstrapTLSMode: "strict", ReleaseVersion: "audit-acceptance", ExpiresAt: expiry, TrustBundleEpoch: 1, TrustBundleSHA256: strings.Repeat("a", 64), BootstrapSHA256: strings.Repeat("b", 64), InstallerSHA256: strings.Repeat("c", 64), CapabilityWarnings: []string{}}}}
	result.OneTimeResultKind = "connector_install_command"
	return result, nil
}

func TestPostgresAuditBrowserAcceptance(t *testing.T) {
	if os.Getenv("ARGUS_AUDIT_BROWSER_E2E") != "1" {
		t.Skip("requires Playwright and the disposable audit PostgreSQL namespace")
	}
	f := newAuditWorkflowFixture(t)
	failedRef, failedID := f.prepare("audit-failure")
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.user)
	if err := f.run(failedID); err != nil {
		t.Fatal(err)
	}
	commands := []string{}
	f.executor.Resources = auditCommandFixture{f.domain}
	for range 4 {
		_, id := f.prepare("audit-browser-target")
		if err := f.run(id); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, id.String())
	}
	f.session()
	f.call("GET", "/enterprise/auth/session", nil, 200, "")
	f.executor.Resources = f.domain
	f.startWorker()
	if err := f.store.InTx(t.Context(), func(q *db.Queries) error {
		for _, item := range []struct{ action, kind string }{{"telemetry.tunnel.disconnect", "telemetry_tunnel"}, {"monitoring.collection.create", "collection"}} {
			if _, err := audit.Append(t.Context(), q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: f.enterprise, Valid: true}, ActorType: "system", ActorID: "acceptance", Action: item.action, ResourceType: item.kind, ResourceID: uuid.NewString(), Result: "success", Details: map[string]any{"host_id": f.host}}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(t.TempDir(), "browser-fixture.json")
	fixture, _ := json.Marshal(map[string]any{"api": f.server.URL + "/api/v1", "token": f.token, "csrf": f.csrf, "hostId": f.host, "failedRef": failedRef, "commands": commands})
	if err = os.WriteFile(fixturePath, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "node", filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "--config=playwright.audit.config.ts", "--workers=1")
	command.Dir = filepath.Join(root, "web", "apps", "enterprise")
	command.Env = append(os.Environ(), "ARGUS_AUDIT_BROWSER_FIXTURE="+fixturePath, "VITE_API_BASE_URL="+f.server.URL)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("real HTTP browser acceptance", err)
	}
}
