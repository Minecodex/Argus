package argusdev

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/collectormanager"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type plan8GatewayLedger struct {
	Report struct {
		InstanceID string `json:"instance_id"`
		Sequence   int64  `json:"sequence"`
		RetryLimit int64  `json:"retry_limit_seconds"`
	} `json:"report"`
}

// Only an owned, normally installed Collector is affected. This verifies the
// real runtime ledger across a Pod UID replacement; the caller subsequently
// produces a new workload log and verifies its exact application query result.
func (a *App) plan8KubernetesStateRestart(ctx context.Context, env *E2EEnvironment, collector uuid.UUID) error {
	namespace := collectormanager.KubernetesNamespace
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil || string(ns.UID) != env.State.Values["p8_collector_namespace_uid"] || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.New("Gateway restart requires this run's exact telemetry namespace UID")
	}
	claim, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, collectormanager.KubernetesGatewayStateClaim, metav1.GetOptions{})
	if err != nil || claim.Labels["argus.io/collector-id"] != collector.String() || claim.Status.Phase != corev1.ClaimBound || claim.Spec.VolumeName == "" {
		return errors.New("actual Gateway state claim is not owned and bound")
	}
	selector := "app.kubernetes.io/name=argus-otelcol-gateway,argus.io/collector-id=" + collector.String()
	read := func(ctx context.Context, pod *corev1.Pod) (plan8GatewayLedger, error) {
		var ledger plan8GatewayLedger
		// The Collector image is distroless. A run-owned unprivileged observer
		// reads only this journal file through a read-only subPath, never the
		// identity directory, certificates or queue. It does not alter runtime.
		image := env.State.FixtureImages["remote-runtime"]
		if image == "" {
			return ledger, errors.New("owned ledger observer image missing")
		}
		name := "plan8-ledger-" + uuid.NewString()[:8]
		no, yes, uid, zero := false, true, int64(10001), int64(0)
		reader, err := env.Kube.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/name": "plan8-ledger-observer"}}, Spec: corev1.PodSpec{
			NodeName: pod.Spec.NodeName, AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever, TerminationGracePeriodSeconds: &zero,
			SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &yes, RunAsUser: &uid, RunAsGroup: &uid},
			Volumes:         []corev1.Volume{{Name: "journal", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim.Name, ReadOnly: true}}}},
			Containers: []corev1.Container{{Name: "read", Image: image, ImagePullPolicy: corev1.PullNever, Command: []string{"/bin/sh", "-c", "exec sleep 120"},
				VolumeMounts:    []corev1.VolumeMount{{Name: "journal", MountPath: "/delivery-state.json", SubPath: "delivery-state.json", ReadOnly: true}},
				SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
				Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("16Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")}}}},
		}}, metav1.CreateOptions{})
		if err != nil {
			return ledger, err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			readerUID := reader.UID
			_ = env.Kube.Client.CoreV1().Pods(namespace).Delete(cleanup, reader.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &readerUID}})
		}()
		if err = env.Kube.WaitPodReady(ctx, namespace, reader.Name, time.Minute); err != nil {
			return ledger, err
		}
		raw, err := env.Kube.execPod(ctx, namespace, reader.Name, "read", "cat", "/delivery-state.json")
		if err != nil || len(raw) > 768<<10 || json.Unmarshal([]byte(raw), &ledger) != nil {
			return ledger, errors.New("actual Gateway delivery ledger unavailable")
		}
		id, err := hex.DecodeString(ledger.Report.InstanceID)
		if err != nil || len(id) != 16 || ledger.Report.Sequence < 1 || ledger.Report.RetryLimit != 300 {
			return ledger, errors.New("actual Gateway ledger has an invalid durable identity")
		}
		return ledger, nil
	}
	get := func(ctx context.Context, previous string) (*corev1.Pod, plan8GatewayLedger, error) {
		pods, err := env.Kube.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return nil, plan8GatewayLedger{}, err
		}
		var selected *corev1.Pod
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil || string(pod.UID) == previous {
				continue
			}
			ready := false
			for _, c := range pod.Status.Conditions {
				ready = ready || c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue
			}
			if ready {
				if selected != nil {
					return nil, plan8GatewayLedger{}, errors.New("ambiguous ready Gateway ownership")
				}
				selected = pod.DeepCopy()
			}
		}
		if selected == nil {
			return nil, plan8GatewayLedger{}, errors.New("actual Gateway not yet ready")
		}
		persistent := false
		for _, v := range selected.Spec.Volumes {
			persistent = persistent || v.Name == "identity-state" && v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == claim.Name
		}
		if !persistent {
			return nil, plan8GatewayLedger{}, errors.New("actual Gateway omitted its persistent state mount")
		}
		ledger, err := read(ctx, selected)
		return selected, ledger, err
	}
	waitReady := func(previous string) (*corev1.Pod, plan8GatewayLedger, error) {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		for {
			pod, ledger, err := get(bounded, previous)
			if err == nil {
				return pod, ledger, nil
			}
			if err = waitContext(bounded, time.Second); err != nil {
				return nil, plan8GatewayLedger{}, err
			}
		}
	}
	before, beforeLedger, err := waitReady("")
	if err != nil {
		return err
	}
	uid, zero := before.UID, int64(0)
	if err = env.Kube.Client.CoreV1().Pods(namespace).Delete(ctx, before.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return err
	}
	after, afterLedger, err := waitReady(string(before.UID))
	if err != nil {
		return err
	}
	current, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, claim.Name, metav1.GetOptions{})
	if err != nil || current.UID != claim.UID || current.Spec.VolumeName != claim.Spec.VolumeName || afterLedger.Report.InstanceID != beforeLedger.Report.InstanceID || afterLedger.Report.Sequence < beforeLedger.Report.Sequence {
		return errors.New("Gateway Pod replacement lost or switched the actual durable ledger")
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "verification_level": "formally_installed_native_collector_gateway_pod_replacement_and_actual_delivery_ledger", "collector_id": collector, "namespace_uid": ns.UID, "claim_uid": claim.UID, "physical_volume": claim.Spec.VolumeName, "before_pod_uid": before.UID, "after_pod_uid": after.UID, "before_ledger": beforeLedger.Report, "after_ledger": afterLedger.Report, "pending_export_batch_survival_verified": false, "following_gate": "new_workload_marker_must_reach_authorized_application_log_query"}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-gateway-state-restart.json"), proof)
}
