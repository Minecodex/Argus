package argusdev

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestM7HostTargetTracksCollectorIdentityAcrossHAReplicaOrdering(t *testing.T) {
	a := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	b := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Status: a.Status}
	for _, pods := range [][]corev1.Pod{{a, b}, {b, a}} {
		name, err := selectM7CollectorPod(pods, "collector-id", func(name string) (string, error) {
			if name == "b" {
				return "collector-id\n", nil
			}
			return "another-collector", nil
		})
		if err != nil || name != "b" {
			t.Fatalf("selected unrelated replica: %s / %v", name, err)
		}
	}
	for _, value := range []string{"collector-id", "missing"} {
		if _, err := selectM7CollectorPod([]corev1.Pod{a, b}, "collector-id", func(string) (string, error) { return value, nil }); err == nil {
			t.Fatal("ambiguous or absent Host Collector was accepted")
		}
	}
}
