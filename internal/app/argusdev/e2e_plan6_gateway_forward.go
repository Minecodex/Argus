package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func replacePlan6ConnectorForward(old *KubeForward, start func([]string) (*KubeForward, error)) (*KubeForward, error) {
	if old == nil || len(old.Ports) != 1 || old.Ports[0].Local == 0 || old.Ports[0].Remote != 9443 {
		return nil, fmt.Errorf("connector forward identity is incomplete")
	}
	// A retired Pod can already have closed the stream. Starting on the same
	// local port below proves the old listener was released; keep the original
	// endpoint and all TLS identity settings unchanged.
	_ = old.Stop()
	current, err := start([]string{fmt.Sprintf("%d:9443", old.Ports[0].Local)})
	if err != nil {
		return nil, err
	}
	if len(current.Ports) != 1 || current.Ports[0] != old.Ports[0] {
		_ = current.Stop()
		return nil, fmt.Errorf("connector forward changed the accepted local endpoint")
	}
	return current, nil
}

func (a *App) refreshPlan6ConnectorForward(ctx context.Context, env *E2EEnvironment) error {
	if env.ConnectorForward == nil {
		return nil
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("connector forward namespace ownership is not established: %v", err)
	}
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-connector-gateway", metav1.GetOptions{})
	if err != nil || deployment.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("connector forward deployment ownership is not established: %v", err)
	}
	old := env.ConnectorForward
	current, err := replacePlan6ConnectorForward(old, func(ports []string) (*KubeForward, error) {
		return env.Kube.PortForwardService(ctx, env.SystemNS, "argus-connector-gateway-public", ports, io.Discard)
	})
	if err != nil {
		return err
	}
	for index, forward := range env.Forwards {
		if forward == old {
			env.Forwards[index] = current
		}
	}
	env.ConnectorForward = current
	report, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "namespace_uid": ns.UID,
		"deployment_uid": deployment.UID, "local_endpoint": env.Endpoints.ConnectorDialAddress,
		"tls_endpoint": env.Endpoints.ConnectorGateway, "same_local_port": true, "tls_verification_unchanged": true}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan6-gateway-forward.json"), report)
}
