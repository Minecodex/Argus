package kubernetesruntime

import (
	"context"
	"errors"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func (b *kubernetesRuntimeBridge) createCommandPod(ctx context.Context, binding *connectorv1.RemoteExecutionBatch) (*corev1.Pod, error) {
	namespace, image := b.options.Namespace, b.options.Image
	if namespace == "" || len(validation.IsDNS1123Label(namespace)) != 0 || image == "" {
		return nil, errors.New("REMOTE_KUBECTL_ENVIRONMENT_UNAVAILABLE")
	}
	name := "argus-remote-" + binding.SessionGeneration
	labels := map[string]string{"argus.io/remote-session": binding.SessionId, "argus.io/remote-generation": binding.SessionGeneration, "argus.io/resource": binding.ResourceId}
	// Install deny-all before the Pod exists. The only cluster connection is the
	// credential-free proxy multiplexed over its private exec stream.
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: labels}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}}}
	createdPolicy, err := b.api.NetworkingV1().NetworkPolicies(namespace).Create(ctx, policy, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	if err != nil {
		return nil, errors.New("REMOTE_KUBECTL_ENVIRONMENT_UNAVAILABLE")
	}
	no := false
	yes := true
	uid := int64(65532)
	grace := int64(5)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever, TerminationGracePeriodSeconds: &grace, SecurityContext: &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Volumes: []corev1.Volume{{Name: "exchange", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: resource.NewQuantity(256<<20, resource.BinarySI)}}}}, Containers: []corev1.Container{{Name: "commands", Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/bin/bash", "-c", "exec sleep infinity"}, WorkingDir: "/tmp", SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, ReadOnlyRootFilesystem: &yes, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}, VolumeMounts: []corev1.VolumeMount{{Name: "exchange", MountPath: "/tmp"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")}}}}}}
	created, err := b.api.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		_ = b.api.NetworkingV1().NetworkPolicies(namespace).Delete(context.WithoutCancel(ctx), name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &createdPolicy.UID}})
		if apierrors.IsAlreadyExists(err) {
			return nil, errors.New("REMOTE_CONTEXT_LOST")
		}
		return nil, errors.New("REMOTE_KUBECTL_ENVIRONMENT_UNAVAILABLE")
	}
	ready := false
	defer func() {
		if !ready {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = b.api.CoreV1().Pods(namespace).Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &created.UID}})
			_ = b.api.NetworkingV1().NetworkPolicies(namespace).Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &createdPolicy.UID}})
		}
	}()
	policy.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: created.Name, UID: created.UID}}
	old, err := b.api.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	for k, v := range labels {
		if old.Labels[k] != v {
			return nil, errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
		}
	}
	old.OwnerReferences = policy.OwnerReferences
	if _, err = b.api.NetworkingV1().NetworkPolicies(namespace).Update(ctx, old, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		current, e := b.api.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil || current.UID != created.UID {
			return nil, errors.New("REMOTE_POD_CHANGED")
		}
		if current.Status.Phase == corev1.PodRunning {
			ready = true
			return current, nil
		}
		if current.Status.Phase == corev1.PodFailed || current.Status.Phase == corev1.PodSucceeded {
			return nil, errors.New("REMOTE_CONTAINER_UNAVAILABLE")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errors.New("REMOTE_KUBECTL_ENVIRONMENT_UNAVAILABLE")
		case <-tick.C:
		}
	}
}

func (b *kubernetesRuntimeBridge) existingCommandPod(ctx context.Context, binding *connectorv1.RemoteExecutionBatch) (*corev1.Pod, error) {
	if b.options.Namespace == "" || len(validation.IsDNS1123Label(b.options.Namespace)) != 0 {
		return nil, errors.New("REMOTE_KUBECTL_ENVIRONMENT_UNAVAILABLE")
	}
	pod, err := b.api.CoreV1().Pods(b.options.Namespace).Get(ctx, "argus-remote-"+binding.SessionGeneration, metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	for key, value := range map[string]string{"argus.io/remote-session": binding.SessionId, "argus.io/remote-generation": binding.SessionGeneration, "argus.io/resource": binding.ResourceId} {
		if pod.Labels[key] != value {
			return nil, errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
		}
	}
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken || len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Name != "commands" || pod.Spec.Containers[0].Image != b.options.Image {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	return pod, nil
}
