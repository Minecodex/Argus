package argusdev

import (
	"context"
	"slices"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// A policy selecting a previously unrestricted Pod would isolate ALL of its
// egress, including the application container sharing this network namespace.
// Add the Collector allowance only when the direction is already isolated.
func supplementPlanV2TracePolicy(ctx context.Context, kube *E2EKube, namespace string, podLabels map[string]string, policy *networkingv1.NetworkPolicy) error {
	existing, err := kube.Client.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	direction := policy.Spec.PolicyTypes[0]
	for _, item := range existing.Items {
		types := item.Spec.PolicyTypes
		if len(types) == 0 {
			types = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
			if len(item.Spec.Egress) > 0 {
				types = append(types, networkingv1.PolicyTypeEgress)
			}
		}
		if !slices.Contains(types, direction) {
			continue
		}
		selector, e := metav1.LabelSelectorAsSelector(&item.Spec.PodSelector)
		if e != nil {
			return e
		}
		if selector.Matches(labels.Set(podLabels)) {
			_, err = kube.Client.NetworkingV1().NetworkPolicies(namespace).Create(ctx, policy, metav1.CreateOptions{})
			return err
		}
	}
	return nil
}
