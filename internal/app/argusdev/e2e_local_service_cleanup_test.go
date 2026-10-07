package argusdev

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestUnallocatedLocalServiceProofRejectsForeignAndAllocatedResources(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Annotations: map[string]string{"meta.helm.sh/release-name": "test-platform", "meta.helm.sh/release-namespace": "test-ns"}}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	if !ownedUnallocatedService(svc, "test-ns", "test") {
		t.Fatal("owned unallocated service was rejected")
	}
	if ownedUnallocatedService(svc, "formal-ns", "test") || ownedUnallocatedService(svc, "test-ns", "formal") {
		t.Fatal("foreign service accepted")
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "192.0.2.1"}}
	if ownedUnallocatedService(svc, "test-ns", "test") {
		t.Fatal("allocated service accepted")
	}
	svc.Status.LoadBalancer.Ingress = nil
	svc.Spec.ExternalIPs = []string{"192.0.2.1"}
	if ownedUnallocatedService(svc, "test-ns", "test") {
		t.Fatal("external service accepted")
	}
}
