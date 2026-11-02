package argusdev

import (
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func plan11ProfileFixture(t *testing.T) (*E2EEnvironment, *plan11DeploymentState, *fake.Clientset) {
	t.Helper()
	one := int32(1)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "owned", UID: "ns-one", Labels: map[string]string{"argus.io/release-id": "comparison"}}}
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-worker", Namespace: ns.Name, UID: "worker-one", Labels: map[string]string{"argus.io/release-id": "comparison"}}, Spec: appsv1.DeploymentSpec{Replicas: &one, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "argus-worker", Image: "candidate@sha256:original", Env: []corev1.EnvVar{{Name: "KEEP", Value: "preserved"}, {Name: "ARGUS_TOOL_IMPLEMENTATION_REVISION", Value: "original"}}}}}}}}
	client := fake.NewClientset(ns, d)
	env := &E2EEnvironment{ReleaseID: "comparison", Kube: &E2EKube{Client: client}}
	state, err := plan11CaptureDeployment(t.Context(), env, ns.Name, d.Name)
	if err != nil {
		t.Fatal(err)
	}
	return env, state, client
}

func TestPlan11ProfileRetriesStatusConflictAndRechecksOperatorImage(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(map[bool]string{false: "status_conflict", true: "operator_replaced_image"}[replacement], func(t *testing.T) {
			env, state, client := plan11ProfileFixture(t)
			updates := 0
			client.PrependReactor("update", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
				updates++
				if updates != 1 {
					return false, nil, nil
				}
				if replacement {
					resource := appsv1.SchemeGroupVersion.WithResource("deployments")
					object, _ := client.Tracker().Get(resource, "owned", "argus-worker")
					deployment := object.(*appsv1.Deployment).DeepCopy()
					deployment.Spec.Template.Spec.Containers[0].Image = "operator-image"
					if err := client.Tracker().Update(resource, deployment, "owned"); err != nil {
						t.Fatal(err)
					}
				}
				return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "argus-worker", errors.New("concurrent status update"))
			})
			err := plan11EditDeployment(t.Context(), env, state, func(d *appsv1.Deployment, index int) error {
				d.Spec.Template.Spec.Containers[index].Image = "legacy-image"
				return nil
			})
			deployment, _ := client.AppsV1().Deployments("owned").Get(t.Context(), "argus-worker", metav1.GetOptions{})
			if replacement {
				if err == nil || updates != 1 || deployment.Spec.Template.Spec.Containers[0].Image != "operator-image" {
					t.Fatal("retry overwrote an operator image", err, updates)
				}
			} else if err != nil || updates != 2 || state.ExpectedImage != "legacy-image" {
				t.Fatal("routine resourceVersion conflict was not retried", err, updates)
			}
		})
	}
}

func TestPlan11ProfileMutationRejectsReplacementAndOperatorChanges(t *testing.T) {
	for _, change := range []string{"namespace_uid", "deployment_uid", "owner", "image"} {
		t.Run(change, func(t *testing.T) {
			env, state, client := plan11ProfileFixture(t)
			ns, _ := client.CoreV1().Namespaces().Get(t.Context(), "owned", metav1.GetOptions{})
			d, _ := client.AppsV1().Deployments("owned").Get(t.Context(), "argus-worker", metav1.GetOptions{})
			switch change {
			case "namespace_uid":
				ns.UID = types.UID("replacement")
				_, _ = client.CoreV1().Namespaces().Update(t.Context(), ns, metav1.UpdateOptions{})
			case "deployment_uid":
				d.UID = types.UID("replacement")
			case "owner":
				d.Labels["argus.io/release-id"] = "other"
			case "image":
				d.Spec.Template.Spec.Containers[0].Image = "operator-image"
			}
			if change != "namespace_uid" {
				_, _ = client.AppsV1().Deployments("owned").Update(t.Context(), d, metav1.UpdateOptions{})
			}
			called := false
			if err := plan11EditDeployment(t.Context(), env, state, func(*appsv1.Deployment, int) error { called = true; return nil }); err == nil || called {
				t.Fatal("profile transaction crossed a changed ownership or image boundary")
			}
		})
	}
}

func TestPlan11ProfileTracksOwnImageAndPreservesOriginalAndOtherEnv(t *testing.T) {
	env, state, client := plan11ProfileFixture(t)
	if err := plan11EditDeployment(t.Context(), env, state, func(d *appsv1.Deployment, i int) error {
		d.Spec.Template.Spec.Containers[i].Image = "legacy@sha256:legacy"
		plan11CopyRevisionEnv(&d.Spec.Template.Spec.Containers[i], []corev1.EnvVar{{Name: "ARGUS_TOOL_IMPLEMENTATION_REVISION", Value: "legacy"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if state.ExpectedImage != "legacy@sha256:legacy" || state.Original.Spec.Template.Spec.Containers[0].Image != "candidate@sha256:original" {
		t.Fatal("profile transaction lost its immutable original")
	}
	d, _ := client.AppsV1().Deployments("owned").Get(t.Context(), "argus-worker", metav1.GetOptions{})
	if len(d.Spec.Template.Spec.Containers[0].Env) != 2 || d.Spec.Template.Spec.Containers[0].Env[0].Value != "preserved" {
		t.Fatal("profile changed unrelated environment")
	}
	if err := plan11EditDeployment(t.Context(), env, state, func(d *appsv1.Deployment, i int) error {
		d.Spec.Template.Spec.Containers[i].Image = state.Original.Spec.Template.Spec.Containers[0].Image
		plan11CopyRevisionEnv(&d.Spec.Template.Spec.Containers[i], state.Original.Spec.Template.Spec.Containers[0].Env)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if state.ExpectedImage != "candidate@sha256:original" {
		t.Fatal("profile could not restore its own original image")
	}
}
