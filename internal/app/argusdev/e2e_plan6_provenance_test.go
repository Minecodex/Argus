package argusdev

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPlan6PodImageVerificationExcludesOnlyRetiredPods(t *testing.T) {
	now := metav1.Now()
	for _, tc := range []struct {
		name     string
		phase    corev1.PodPhase
		deleting bool
		want     bool
	}{
		{"current running", corev1.PodRunning, false, true},
		{"terminating old replica", corev1.PodRunning, true, false},
		{"completed fixture", corev1.PodSucceeded, false, false},
		{"failed fixture", corev1.PodFailed, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := corev1.Pod{Status: corev1.PodStatus{Phase: tc.phase}}
			if tc.deleting {
				pod.DeletionTimestamp = &now
			}
			if got := plan6PodRequiresImageVerification(pod); got != tc.want {
				t.Fatalf("requires verification = %v, want %v", got, tc.want)
			}
		})
	}
}
