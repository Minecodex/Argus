package collectormanager

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

func TestLiveKubernetesGatewayStatePVCReplacesPodAndCleansOwnedVolume(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_K8S_STATE_LIVE") != "1" {
		t.Skip("requires explicit disposable cluster kubeconfig and fresh telemetry namespace")
	}
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("ARGUS_PLAN8_K8S_STATE_KUBECONFIG"))
	if err != nil {
		t.Fatal("disposable kubeconfig unavailable")
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	if _, err = client.CoreV1().Namespaces().Get(ctx, KubernetesNamespace, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("requires a fresh telemetry namespace; refusing existing resources", err)
	}
	classes, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	className := ""
	for _, class := range classes.Items {
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" || class.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			defaults++
			className = class.Name
			if class.ReclaimPolicy == nil || *class.ReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
				t.Fatal("owned storage test requires a default Delete reclaim policy")
			}
		}
	}
	if defaults != 1 {
		t.Fatal("requires one explicit default StorageClass", defaults)
	}
	owner := uuid.NewString()
	ns, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: KubernetesNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/collector-id": owner, "argus.test/owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{"scope": "actual_kubernetes_pvc_unprivileged_filesystem_pod_replacement_not_collector_queue_or_formal_enrollment", "namespace_uid": ns.UID, "storage_class": className}
	volume := ""
	completed := false
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		current, err := client.CoreV1().Namespaces().Get(cleanup, ns.Name, metav1.GetOptions{})
		if err == nil && current.UID == ns.UID && current.Labels["argus.test/owner"] == owner {
			uid := ns.UID
			err = client.CoreV1().Namespaces().Delete(cleanup, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		}
		if err != nil && !apierrors.IsNotFound(err) {
			t.Error("owned namespace cleanup failed", err)
		}
		err = wait.PollUntilContextCancel(cleanup, 200*time.Millisecond, true, func(ctx context.Context) (bool, error) {
			_, nsErr := client.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
			if nsErr == nil {
				return false, nil
			}
			if !apierrors.IsNotFound(nsErr) {
				return false, nsErr
			}
			if volume != "" {
				_, pvErr := client.CoreV1().PersistentVolumes().Get(ctx, volume, metav1.GetOptions{})
				return apierrors.IsNotFound(pvErr), nil
			}
			return true, nil
		})
		if err != nil {
			t.Error("owned namespace or dynamically provisioned volume remains", err)
		}
		proof["cleanup_passed"], proof["passed"] = err == nil, completed && !t.Failed()
		if path := os.Getenv("ARGUS_PLAN8_K8S_STATE_EVIDENCE"); path != "" {
			raw, _ := json.MarshalIndent(proof, "", "  ")
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Error(err)
			}
		}
	})
	if err = ensureKubernetesGatewayState(ctx, client, owner); err != nil {
		t.Fatal(err)
	}
	marker := "state-" + uuid.NewString()
	createPod := func(name string, write bool) *corev1.Pod {
		t.Helper()
		no, yes, uid, zero := false, true, int64(10001), int64(0)
		command := "exec sleep 3600"
		if write {
			command = "umask 077; printf '%s' '" + marker + "' >/state/owned-state; sync; exec sleep 3600"
		}
		pod, err := client.CoreV1().Pods(ns.Name).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.test/owner": owner}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, TerminationGracePeriodSeconds: &zero, RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &yes, RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid},
			Volumes:         []corev1.Volume{{Name: "state", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: KubernetesGatewayStateClaim}}}},
			Containers: []corev1.Container{{Name: "state", Image: "alpine:3.21", ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/bin/sh", "-c", command},
				VolumeMounts: []corev1.VolumeMount{{Name: "state", MountPath: "/state"}}, SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("8Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("32Mi")}}}},
		}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			pod, err = client.CoreV1().Pods(ns.Name).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			t.Fatal("owned state Pod did not become ready", err)
		}
		return pod
	}
	read := func(pod *corev1.Pod) string {
		t.Helper()
		request := client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).Namespace(ns.Name).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "state", Command: []string{"cat", "/state/owned-state"}, Stdout: true, Stderr: true}, scheme.ParameterCodec)
		executor, err := remotecommand.NewSPDYExecutor(config, "POST", request.URL())
		if err != nil {
			t.Fatal(err)
		}
		var output, stderr bytes.Buffer
		if err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &output, Stderr: &stderr}); err != nil {
			t.Fatal("owned state read failed", err)
		}
		return output.String()
	}
	first := createPod("state-before", true)
	if read(first) != marker {
		t.Fatal("initial unprivileged volume write was not readable")
	}
	claim, err := client.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, KubernetesGatewayStateClaim, metav1.GetOptions{})
	if err != nil || claim.Status.Phase != corev1.ClaimBound || claim.Spec.VolumeName == "" {
		t.Fatal("default dynamic storage did not bind", err)
	}
	volume = claim.Spec.VolumeName
	uid, zero := first.UID, int64(0)
	if err = client.CoreV1().Pods(ns.Name).Delete(ctx, first.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Pods(ns.Name).Get(ctx, first.Name, metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	}); err != nil {
		t.Fatal(err)
	}
	second := createPod("state-after", false)
	if second.UID == first.UID || read(second) != marker {
		t.Fatal("replacement Pod did not recover the exact persisted state")
	}
	current, err := client.CoreV1().PersistentVolumeClaims(ns.Name).Get(ctx, KubernetesGatewayStateClaim, metav1.GetOptions{})
	if err != nil || current.UID != claim.UID || current.Spec.VolumeName != volume {
		t.Fatal("Pod replacement switched physical claims", err)
	}
	proof["before_pod_uid"], proof["after_pod_uid"], proof["claim_uid"], proof["volume"] = first.UID, second.UID, claim.UID, volume
	proof["node_before"], proof["node_after"], proof["run_as_uid"] = first.Spec.NodeName, second.Spec.NodeName, 10001
	if len(second.Status.ContainerStatuses) > 0 {
		proof["actual_image_id"] = second.Status.ContainerStatuses[0].ImageID
	}
	completed = true
}
