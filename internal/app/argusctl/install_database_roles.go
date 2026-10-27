package argusctl

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func (a *App) reconcileInstallationRuntimeRoles(ctx context.Context, clients *kubeClients, cfg *InstallConfig, installationCredentials map[string]string) error {
	forward, err := portForwardService(ctx, clients, cfg.Spec.Namespaces.System, "argus-postgresql", 5432)
	if err != nil {
		return fmt.Errorf("connect for runtime database permissions: %w", err)
	}
	defer forward.Stop()
	address := &url.URL{Scheme: "postgres", User: url.UserPassword("argus", installationCredentials["postgresql-password"]), Host: fmt.Sprintf("127.0.0.1:%d", forward.localPort), Path: "/argus", RawQuery: "sslmode=disable"}
	receipt, err := postgres.ReconcileRuntimeRolePermissions(ctx, address.String())
	if err != nil {
		return fmt.Errorf("reconcile runtime database permissions: %w", err)
	}
	_, err = fmt.Fprintf(a.stdout, "Runtime database roles reconciled: policy %s, schema %d, roles %d, sha256 %s\n", receipt.PolicyVersion, receipt.SchemaVersion, len(receipt.Roles), receipt.PolicySHA256)
	return err
}
