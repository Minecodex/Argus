package argusdev

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Each HA Direct Executor replica has its own synthetic Host sidecar. Locate
// the installed Collector by its authoritative identity, never by Pod ordering.
func (a *App) m7HostCollectorPod(ctx context.Context, env *E2EEnvironment) (string, error) {
	value, err := a.postgresQuery(ctx, env, "SELECT id::text FROM collector_instances WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' AND resource_type='host' AND resource_id='"+env.State.Values["m7_host_id"]+"' ORDER BY created_at DESC LIMIT 1;")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(value)
	if id == "" {
		return "", fmt.Errorf("M7 Host Collector identity is missing")
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-direct-executor"})
	if err != nil {
		return "", err
	}
	return selectM7CollectorPod(pods.Items, id, func(name string) (string, error) {
		return env.Kube.execPod(ctx, env.SystemNS, name, "argus-e2e-systemd-host", "cat", "/var/lib/argus-otelcol/.active-collector-id")
	})
}

func selectM7CollectorPod(pods []corev1.Pod, id string, read func(string) (string, error)) (string, error) {
	matched := ""
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		value, err := read(pod.Name)
		if err != nil || strings.TrimSpace(value) != id {
			continue
		}
		if matched != "" {
			return "", fmt.Errorf("M7 Collector identity exists in multiple Host Pods")
		}
		matched = pod.Name
	}
	if matched == "" {
		return "", fmt.Errorf("M7 Collector is not installed in any owned Host Pod")
	}
	return matched, nil
}

func (a *App) execM7Host(ctx context.Context, env *E2EEnvironment, container string, command ...string) (string, error) {
	pod, err := a.m7HostCollectorPod(ctx, env)
	if err != nil {
		return "", err
	}
	return env.Kube.execPod(ctx, env.SystemNS, pod, container, command...)
}
