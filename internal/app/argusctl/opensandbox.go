package argusctl

import (
	"context"
	"fmt"
	"time"
)

func (a *App) installOpenSandbox(ctx context.Context, cfg *InstallConfig, clients *kubeClients, helm helmManager, root string) error {
	if err := clients.setStage(ctx, cfg, "sandbox", "running", "installing OpenSandbox template runtime"); err != nil {
		return err
	}
	controller, err := helm.loadOpenSandboxControllerChart(ctx)
	if err != nil {
		return err
	}
	shared, err := sharedOpenSandboxController(ctx, cfg, clients, controller)
	if err != nil {
		return err
	}
	if shared == "" {
		if err := helm.installOrUpgrade(ctx, cfg.upstreamReleaseName("os"), cfg.Spec.Namespaces.Sandbox, controller, openSandboxControllerValues(cfg)); err != nil {
			return err
		}
		if err := a.markOwnedCRDs(ctx, cfg); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(a.stdout, "Reusing compatible OpenSandbox controller %s; preserving external ownership\n", shared)
	}
	key, err := ensureSecretValue(ctx, clients, cfg.Spec.Namespaces.Sandbox, cfg.Spec.ReleaseID+"-generated-secrets", "opensandbox-api-key", 32)
	if err != nil {
		return err
	}
	chart, err := loadLocalChart(root, "argus-sandbox")
	if err != nil {
		return err
	}
	if err := helm.installOrUpgrade(ctx, cfg.Spec.ReleaseID+"-sandbox", cfg.Spec.Namespaces.Sandbox, chart, sandboxValues(cfg, key)); err != nil {
		return err
	}
	for _, name := range []string{"opensandbox-controller-manager", "opensandbox-server"} {
		if shared != "" && name == "opensandbox-controller-manager" {
			continue
		}
		if err := waitForDeployment(ctx, clients, cfg.Spec.Namespaces.Sandbox, name, 10*time.Minute); err != nil {
			return err
		}
	}
	return clients.setStage(ctx, cfg, "sandbox", "complete", "OpenSandbox template runtime ready")
}
func openSandboxControllerValues(cfg *InstallConfig) map[string]any {
	return map[string]any{"namespaceOverride": cfg.Spec.Namespaces.Sandbox, "controller": map[string]any{"replicaCount": 1, "image": map[string]any{"repository": "opensandbox/controller", "tag": "v0.2.0", "pullPolicy": "IfNotPresent"},
		"resources": map[string]any{"requests": map[string]any{"cpu": "25m", "memory": "64Mi"}, "limits": map[string]any{"cpu": "500m", "memory": "256Mi"}}}}
}
func openSandboxConfig(cfg *InstallConfig) string {
	return fmt.Sprintf(`[server]
host = "0.0.0.0"
port = 8080
api_key = ""
[log]
level = "INFO"
[runtime]
type = "kubernetes"
execd_image = "opensandbox/execd:v1.0.22@sha256:0d8f44cf4194732719aa79999d4b120c98bdab02bc61e9ad13f75f83af4c2684"
[kubernetes]
namespace = %q
informer_enabled = true
informer_resync_seconds = 300
informer_watch_timeout_seconds = 60
workload_provider = "batchsandbox"
batchsandbox_template_file = "/etc/opensandbox/batchsandbox.yaml"
[egress]
image = %q
mode = "dns+nft"
disable_ipv6 = false
`, cfg.Spec.Namespaces.Sandbox, cfg.Image("argus-workspace-egress"))
}
