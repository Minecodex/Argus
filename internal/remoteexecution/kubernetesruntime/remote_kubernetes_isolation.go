package kubernetesruntime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Merely creating a NetworkPolicy is insufficient on a CNI that ignores it.
// Before any user script is accepted, verify a live, reachable peer is denied.
func (b *kubernetesRuntimeBridge) verifyIsolation(ctx context.Context, commands *corev1.Pod) error {
	fail := func(stage string) error {
		slog.WarnContext(ctx, "Remote network isolation unavailable", "stage", stage, "namespace", commands.Namespace, "pod", commands.Name)
		return errors.New("REMOTE_NETWORK_ISOLATION_UNAVAILABLE")
	}
	labels := map[string]string{"argus.io/isolation-probe": string(commands.UID)}
	port, protocol := intstr.FromInt32(39999), corev1.ProtocolTCP
	selector := metav1.LabelSelector{MatchLabels: labels}
	policy, err := b.api.NetworkingV1().NetworkPolicies(b.namespace).Create(ctx, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: commands.Name + "-probes", Namespace: commands.Namespace, Labels: labels, OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: commands.Name, UID: commands.UID}}}, Spec: networkingv1.NetworkPolicySpec{PodSelector: selector, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{Ports: []networkingv1.NetworkPolicyPort{{Port: &port, Protocol: &protocol}}}}, Egress: []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{PodSelector: &selector}}, Ports: []networkingv1.NetworkPolicyPort{{Port: &port, Protocol: &protocol}}}}}}, metav1.CreateOptions{})
	if err != nil {
		return fail("probe_policy_create")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = b.api.NetworkingV1().NetworkPolicies(b.namespace).Delete(cleanup, policy.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &policy.UID}})
	}()
	peer := commands.DeepCopy()
	peer.ObjectMeta = metav1.ObjectMeta{Name: commands.Name + "-probe", Namespace: commands.Namespace, Labels: map[string]string{"argus.io/isolation-probe": string(commands.UID)}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: commands.Name, UID: commands.UID}}}
	peer.Status = corev1.PodStatus{}
	peer.Spec.Containers[0].Command = []string{"/usr/local/bin/argus-remote-runtime", "--isolation-sentinel"}
	created, err := b.api.CoreV1().Pods(b.namespace).Create(ctx, peer, metav1.CreateOptions{})
	if err != nil {
		return fail("sentinel_create")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = b.api.CoreV1().Pods(b.namespace).Delete(cleanup, created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &created.UID}})
	}()
	ready, err := b.waitRunningPod(ctx, created, 45*time.Second)
	if err != nil || net.ParseIP(readyIP(ready)) == nil {
		return fail("sentinel_ready")
	}
	witness := peer.DeepCopy()
	witness.Name = commands.Name + "-witness"
	witness.Spec.Containers[0].Command = []string{"/bin/bash", "-c", "exec sleep infinity"}
	witnessPod, err := b.api.CoreV1().Pods(b.namespace).Create(ctx, witness, metav1.CreateOptions{})
	if err != nil {
		return fail("witness_create")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = b.api.CoreV1().Pods(b.namespace).Delete(cleanup, witnessPod.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &witnessPod.UID}})
	}()
	if _, err = b.waitRunningPod(ctx, witnessPod, 45*time.Second); err != nil {
		return fail("witness_ready")
	}
	observer := &kubernetesRuntimeBridge{api: b.api, config: b.config, namespace: b.namespace, pod: witnessPod.Name, container: b.container, podUID: witnessPod.UID}
	address := net.JoinHostPort(ready.Status.PodIP, "39999")
	// Prove real cross-Pod connectivity first. Loopback alone would accept a
	// broken cluster route as evidence that the command Pod is isolated.
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = observer.exec(ctx, []string{"/usr/local/bin/argus-remote-runtime", "--probe-tcp", address}, nil, io.Discard, io.Discard)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fail("positive_control_unreachable")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	err = b.exec(ctx, []string{"/usr/local/bin/argus-remote-runtime", "--probe-tcp", address}, nil, io.Discard, io.Discard)
	// Exit 1 is the helper's explicit TCP-denied result. Exec/channel failures
	// are not evidence of isolation.
	var exit interface{ ExitStatus() int }
	if !errors.As(err, &exit) || exit.ExitStatus() != 1 {
		return fail("command_pod_not_denied")
	}
	return nil
}
func readyIP(p *corev1.Pod) string {
	if p == nil {
		return ""
	}
	return p.Status.PodIP
}
func (b *kubernetesRuntimeBridge) waitRunningPod(ctx context.Context, expected *corev1.Pod, timeout time.Duration) (*corev1.Pod, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		pod, err := b.api.CoreV1().Pods(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
		if err != nil || pod.UID != expected.UID {
			return nil, errors.New("REMOTE_POD_CHANGED")
		}
		if pod.Status.Phase == corev1.PodRunning {
			for _, s := range pod.Status.ContainerStatuses {
				if s.State.Running != nil {
					return pod, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("REMOTE_CONTAINER_UNAVAILABLE")
		case <-tick.C:
		}
	}
}
