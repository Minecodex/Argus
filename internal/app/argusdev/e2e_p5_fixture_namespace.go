package argusdev

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (env *E2EEnvironment) ReplayNamespace() string {
	if env.Options.Suite == "p5" {
		return env.ReleaseID + "-customer-mcp"
	}
	return env.SandboxNS
}

// Reinstallation protects every service in Argus's infrastructure namespaces.
// A customer MCP fixture must therefore have its own separately owned plane.
func prepareReplayFixtureNamespace(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.Suite != "p5" {
		return nil
	}
	if env.ReleaseID == "" {
		return fmt.Errorf("P5 customer fixture requires a release identity")
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: env.ReplayNamespace(), Labels: map[string]string{"app.kubernetes.io/part-of": "argus-e2e", "argus.io/release-id": env.ReleaseID}}}
	if _, err := env.Kube.Client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create owned customer fixture namespace: %w", err)
	}
	env.ManagedNamespaces = append(env.ManagedNamespaces, namespace.Name)
	return nil
}
