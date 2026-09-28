package argusdev

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestForwardAccessUsesRealServiceAddressesWithoutCloudLoadBalancer(t *testing.T) {
	client := fake.NewClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "forward"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.2"}}, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "argus-connector-gateway-public", Namespace: "system"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ClusterIP: "10.0.0.3"}})
	env := &E2EEnvironment{SystemNS: "system", Kube: &E2EKube{Client: client}}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	a, b, err := resolveE2EAddresses(ctx, env, "forward/ingress")
	if err != nil || a != "10.0.0.2" || b != "10.0.0.3" {
		t.Fatalf("forwarding waited on nonexistent LB or chose wrong route: %s/%s %v", a, b, err)
	}
	if _, _, err := resolveE2EAddresses(ctx, env, "forward/ingress/other"); err == nil {
		t.Fatal("invalid service identity accepted")
	}
}

func TestUnforwardedAccessStillUsesAllocatedLoadBalancer(t *testing.T) {
	client := fake.NewClientset(&networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "argus-web", Namespace: "system"}, Status: networkingv1.IngressStatus{LoadBalancer: networkingv1.IngressLoadBalancerStatus{Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "192.0.2.10"}}}}}, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "argus-connector-gateway-public", Namespace: "system"}, Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.3"}, Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.11"}}}}})
	a, b, err := resolveE2EAddresses(t.Context(), &E2EEnvironment{SystemNS: "system", Kube: &E2EKube{Client: client}}, "")
	if err != nil || a != "192.0.2.10" || b != "192.0.2.11" {
		t.Fatalf("ordinary LB route changed: %s/%s %v", a, b, err)
	}
}
