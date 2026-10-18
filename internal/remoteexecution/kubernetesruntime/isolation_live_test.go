package kubernetesruntime

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLiveIsolationRequiresPositiveControlAndRejectsConflictingAllow(t *testing.T) {
	f := newLiveKubeFixture(t)
	bridge, err := openKubernetesRuntime(t.Context(), liveKubeBinding(f, "kubectl"), f.credential)
	if err != nil {
		t.Fatal("positive witness and denied command Pod preflight failed", err)
	}
	bridge.Close()
	if err = f.api.CoreV1().Pods(f.namespace).Delete(t.Context(), bridge.pod, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &bridge.podUID}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.api.NetworkingV1().NetworkPolicies(f.namespace).Create(t.Context(), &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "conflicting-allow"}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{}}, Egress: []networkingv1.NetworkPolicyEgressRule{{}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if other, err := openKubernetesRuntime(t.Context(), liveKubeBinding(f, "kubectl"), f.credential); err == nil {
		other.Close()
		t.Fatal("conflicting allow policy bypassed isolation")
	} else if err.Error() != "REMOTE_NETWORK_ISOLATION_UNAVAILABLE" {
		t.Fatal("wrong isolation failure", err)
	}
}
