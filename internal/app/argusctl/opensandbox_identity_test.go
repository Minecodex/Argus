package argusctl

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSharedControllerDigestAndOwnership(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong_digest", "wrong_namespace", "wrong_owner", "missing_manager", "not_ready", "no_pods", "rolling_out"} {
		t.Run(scenario, func(t *testing.T) {
			controller := true
			one := int32(1)
			digest := "sha256:" + strings.Repeat("a", 64)
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opensandbox-controller-manager", Namespace: "external", UID: "controller"}, Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"controller": "shared"}}}, Status: appsv1.DeploymentStatus{UpdatedReplicas: 1, ReadyReplicas: 1}}
			rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "external", UID: "rs", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", UID: dep.UID, Controller: &controller}}}}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "external", Labels: map[string]string{"controller": "shared"}, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "rs", UID: rs.UID, Controller: &controller}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "manager", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, ImageID: "mirror.example/controller@" + digest}}}}
			pin := &SharedSandboxController{Namespace: "external", ImageDigest: digest}
			switch scenario {
			case "wrong_digest":
				pod.Status.ContainerStatuses[0].ImageID = "mirror.example/controller@sha256:" + strings.Repeat("b", 64)
			case "wrong_namespace":
				pin.Namespace = "other"
			case "wrong_owner":
				rs.OwnerReferences[0].UID = "unrelated"
			case "missing_manager":
				pod.Status.ContainerStatuses[0].Name = "sidecar"
			case "not_ready":
				pod.Status.ContainerStatuses[0].Ready = false
			case "rolling_out":
				dep.Status.UpdatedReplicas = 0
			case "no_pods":
				pod.Labels = map[string]string{"controller": "other"}
			}
			typed := fake.NewClientset(dep, rs, pod)
			err := verifySharedSandboxController(context.Background(), &kubeClients{typed: typed}, dep, pin)
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("scenario %s: %v", scenario, err)
			}
			for _, action := range typed.Actions() {
				if action.GetVerb() != "get" && action.GetVerb() != "list" {
					t.Fatalf("external resource mutated: %s", action.GetVerb())
				}
			}
		})
	}
}

func TestSharedControllerPinValidation(t *testing.T) {
	for _, pin := range []*SharedSandboxController{{Namespace: "external", ImageDigest: "latest"}, {Namespace: "../other", ImageDigest: "sha256:" + strings.Repeat("a", 64)}} {
		if err := pin.validate(); err == nil {
			t.Fatal("invalid shared controller pin accepted")
		}
	}
}
