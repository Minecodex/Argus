package argusctl

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestArtifactRouteSelectsIngressClassOwner(t *testing.T) {
	labels := map[string]string{"app.kubernetes.io/name": "ingress-nginx", "app.kubernetes.io/component": "controller", "app.kubernetes.io/instance": "selected"}
	class := &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: "nginx", Labels: labels, Annotations: map[string]string{"meta.helm.sh/release-namespace": "entry"}}, Spec: networkingv1.IngressClassSpec{Controller: "k8s.io/ingress-nginx"}}
	primary := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "entry", Labels: labels}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 443, TargetPort: intstr.FromString("https")}}}}
	admission := primary.DeepCopy()
	admission.Name = "admission"
	admission.Spec.Ports[0].TargetPort = intstr.FromString("webhook")
	other := primary.DeepCopy()
	other.Namespace = "another-project"
	clients := &kubeClients{typed: fake.NewClientset(class, primary, admission, other)}
	cfg := &InstallConfig{}
	cfg.Spec.Exposure.IngressClassName = "nginx"
	address, err := httpsInternalAddress(context.Background(), clients, cfg)
	if err != nil || address != "controller.entry.svc:443" {
		t.Fatalf("selected %q: %v", address, err)
	}
}
