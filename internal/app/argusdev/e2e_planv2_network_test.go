package argusdev

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTraceSidecarDoesNotIntroduceApplicationNetworkIsolation(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		client := fake.NewClientset()
		kube := &E2EKube{Client: client}
		selector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "query"}}
		if isolated {
			_, _ = client.NetworkingV1().NetworkPolicies("owned").Create(context.Background(), &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "existing"}, Spec: networkingv1.NetworkPolicySpec{PodSelector: selector, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}}}, metav1.CreateOptions{})
		}
		policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "collector-only"}, Spec: networkingv1.NetworkPolicySpec{PodSelector: selector, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}}}
		if err := supplementPlanV2TracePolicy(context.Background(), kube, "owned", map[string]string{"app": "query"}, policy); err != nil {
			t.Fatal(err)
		}
		_, err := client.NetworkingV1().NetworkPolicies("owned").Get(context.Background(), "collector-only", metav1.GetOptions{})
		if (err == nil) != isolated {
			t.Fatal("sidecar changed original network isolation")
		}
	}
}
