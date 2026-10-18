package argusdev

import (
	"testing"

	"github.com/kakj-go/Argus/internal/app/argusctl"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFormalWorkloadRestoreRejectsReplacementAndConcurrentChanges(t *testing.T) {
	for _, changed := range []string{"namespace_uid", "release", "workload_uid", "replicas"} {
		t.Run(changed, func(t *testing.T) {
			zero := int32(0)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "formal", UID: "namespace-original", Labels: map[string]string{"argus.io/release-id": "formal-release"}}}
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-worker", Namespace: ns.Name, UID: "worker-original"}, Spec: appsv1.DeploymentSpec{Replicas: &zero}}
			switch changed {
			case "namespace_uid":
				ns.UID = "namespace-replacement"
			case "release":
				ns.Labels["argus.io/release-id"] = "different-release"
			case "workload_uid":
				deployment.UID = "worker-replacement"
			case "replicas":
				one := int32(1)
				deployment.Spec.Replicas = &one
			}
			client := fake.NewClientset(ns, deployment)
			env := &E2EEnvironment{Kube: &E2EKube{Client: client}, Paused: []pausedWorkload{{Namespace: ns.Name, NamespaceUID: "namespace-original", ReleaseID: "formal-release", UID: "worker-original", Name: deployment.Name, Kind: "Deployment", Replicas: 2, Applied: true}}}
			if err := restoreFormalWorkloads(t.Context(), env); err == nil {
				t.Fatal("restore overwrote changed ownership or replicas")
			}
			got, err := client.AppsV1().Deployments(ns.Name).Get(t.Context(), deployment.Name, metav1.GetOptions{})
			if err != nil || *got.Spec.Replicas != *deployment.Spec.Replicas || got.UID != deployment.UID {
				t.Fatal("replacement was modified", err)
			}
		})
	}
}

func TestFormalWorkloadPauseRecordsUIDAndExcludesOtherProjects(t *testing.T) {
	one := int32(1)
	formal := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "formal", UID: "namespace-original", Labels: map[string]string{"argus.io/release-id": "formal-release", "app.kubernetes.io/part-of": "argus"}}}
	other := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "judex", UID: "other-project", Labels: map[string]string{"app.kubernetes.io/part-of": "judex"}}}
	worker := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-worker", Namespace: formal.Name, UID: "worker-original"}, Spec: appsv1.DeploymentSpec{Replicas: &one}}
	judex := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "judex", Namespace: other.Name, UID: "judex-original"}, Spec: appsv1.DeploymentSpec{Replicas: &one}}
	client := fake.NewClientset(formal, other, worker, judex)
	env := &E2EEnvironment{ReleaseID: "temporary-e2e", Options: E2EOptions{Artifacts: t.TempDir()}, Kube: &E2EKube{Client: client}}
	if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	if len(env.Paused) != 1 || env.Paused[0].UID != worker.UID || env.Paused[0].NamespaceUID != formal.UID || !env.Paused[0].Applied {
		t.Fatal("missing ownership record", env.Paused)
	}
	got, err := client.AppsV1().Deployments(formal.Name).Get(t.Context(), worker.Name, metav1.GetOptions{})
	if err != nil || *got.Spec.Replicas != 0 {
		t.Fatal("formal workload not paused", err)
	}
	untouched, err := client.AppsV1().Deployments(other.Name).Get(t.Context(), judex.Name, metav1.GetOptions{})
	if err != nil || *untouched.Spec.Replicas != 1 {
		t.Fatal("other project was paused", err)
	}
}

func TestFormalWorkloadRestoreAcceptsExactStateAlreadyRestoredByOperator(t *testing.T) {
	one := int32(1)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "formal", UID: "namespace-original", Labels: map[string]string{"argus.io/release-id": "formal-release"}}}
	workload := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "argus-kafka", Namespace: ns.Name, UID: "workload-original"}, Spec: appsv1.StatefulSetSpec{Replicas: &one}}
	client := fake.NewClientset(ns, workload)
	env := &E2EEnvironment{Kube: &E2EKube{Client: client}, Paused: []pausedWorkload{{Namespace: ns.Name, NamespaceUID: ns.UID, ReleaseID: "formal-release", UID: workload.UID, Name: workload.Name, Kind: "StatefulSet", Replicas: one, Applied: true}}}
	if err := restoreFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	if env.Paused[0].Applied {
		t.Fatal("restored workload remained marked paused")
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" {
			t.Fatal("already-restored workload was patched")
		}
	}
}

func TestFormalPausePreservesAcceptedSharedControllerDependency(t *testing.T) {
	one := int32(1)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argus-sandbox", UID: "formal-namespace", Labels: map[string]string{"argus.io/release-id": "argus", "app.kubernetes.io/part-of": "argus"}}}
	controller := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opensandbox-controller-manager", Namespace: namespace.Name, UID: "controller"}, Spec: appsv1.DeploymentSpec{Replicas: &one}}
	server := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opensandbox-server", Namespace: namespace.Name, UID: "server"}, Spec: appsv1.DeploymentSpec{Replicas: &one}}
	client := fake.NewClientset(namespace, controller, server)
	env := &E2EEnvironment{ReleaseID: "plan7-temporary", Options: E2EOptions{Artifacts: t.TempDir(), SharedController: &argusctl.SharedSandboxController{Namespace: namespace.Name}}, Kube: &E2EKube{Client: client}}
	if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	if len(env.Paused) != 1 || env.Paused[0].Name != server.Name {
		t.Fatalf("unexpected paused workload set: %+v", env.Paused)
	}
	got, err := client.AppsV1().Deployments(namespace.Name).Get(t.Context(), controller.Name, metav1.GetOptions{})
	if err != nil || *got.Spec.Replicas != 1 {
		t.Fatalf("shared controller paused: %v", err)
	}
}
