package argusdev

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentAvailabilityDoesNotHideDrainingOldProcesses(t *testing.T) {
	one := int32(1)
	deployment := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &one}}
	ready := corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	old := *ready.DeepCopy()
	stamp := metav1.Now()
	old.DeletionTimestamp = &stamp
	if deploymentPodsSettled(deployment, []corev1.Pod{ready, old}) || deploymentPodsSettled(deployment, []corev1.Pod{old}) {
		t.Fatal("a ready replacement hid an old process still draining")
	}
	if !deploymentPodsSettled(deployment, []corev1.Pod{ready}) {
		t.Fatal("the actual settled replacement was rejected")
	}
	ready.Status.Conditions[0].Status = corev1.ConditionFalse
	if deploymentPodsSettled(deployment, []corev1.Pod{ready}) {
		t.Fatal("an unready process satisfied rollout readiness")
	}
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	if deploymentPodsSettled(deployment, []corev1.Pod{old}) || !deploymentPodsSettled(deployment, nil) {
		t.Fatal("zero replicas did not wait for actual old Pod removal")
	}
}
