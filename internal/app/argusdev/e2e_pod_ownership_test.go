package argusdev

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestE2EDeploymentPodOwnershipUsesControllerUIDs(t *testing.T) {
	for _, change := range []string{"none", "foreign_namespace", "replaced_replicaset", "foreign_deployment"} {
		t.Run(change, func(t *testing.T) {
			controller := true
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "owned", Labels: map[string]string{"argus.io/release-id": "run"}}}
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-worker", Namespace: "owned", UID: "deployment"}}
			rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "worker-rs", Namespace: "owned", UID: "replicaset", OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: dep.Name, UID: dep.UID, Controller: &controller}}}}
			pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "owned", Name: "worker-pod", OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: &controller}}}}
			switch change {
			case "foreign_namespace":
				ns.Labels["argus.io/release-id"] = "foreign"
			case "replaced_replicaset":
				rs.UID = "replacement"
			case "foreign_deployment":
				dep.UID = "replacement"
			}
			k := E2EKube{Client: fake.NewSimpleClientset(ns, dep, rs)}
			err := k.VerifyDeploymentPod(t.Context(), "owned", "argus-worker", "run", pod)
			if (err == nil) != (change == "none") {
				t.Fatalf("ownership %s: %v", change, err)
			}
		})
	}
}
