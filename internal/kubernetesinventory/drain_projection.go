package kubernetesinventory

import "reflect"

func validateDrainResult(input Mutation, result MutationResult, dryRun bool) error {
	if input.Operation != "drain" {
		if result.Drain != nil || result.DrainPlan != nil {
			return ErrProtocol
		}
		return nil
	}
	if result.Drain == nil || result.After == nil || result.After.UID != input.Query.UID {
		return ErrProtocol
	}
	plan := input.DrainPlan
	if dryRun && plan == nil {
		plan = result.DrainPlan
	}
	if plan == nil || len(plan.Pods) > drainLimit || len(result.Drain.Pods) != len(plan.Pods) {
		return ErrProtocol
	}
	if result.DrainPlan != nil && !reflect.DeepEqual(result.DrainPlan, plan) {
		return ErrProtocol
	}
	seen := map[string]bool{}
	for i, p := range result.Drain.Pods {
		approved := plan.Pods[i]
		if seen[p.UID] || p.UID == "" || p.UID != approved.UID || p.Name != approved.Name || p.Namespace != approved.Namespace || p.NamespaceUID != approved.NamespaceUID || !reflect.DeepEqual(p.Volumes, approved.Volumes) {
			return ErrProtocol
		}
		seen[p.UID] = true
		if approved.SpecDigest == "" || approved.Name == "" || approved.Namespace == "" || approved.NamespaceUID == "" {
			return ErrProtocol
		}
		if p.State == "skipped" && (p.Reason != approved.Disposition || (p.Reason != "daemonset" && p.Reason != "mirror" && p.Reason != "completed")) {
			return ErrProtocol
		}
		if p.State == "blocked" && p.Reason != "local_storage" && p.Reason != "unmanaged" && p.Reason != "eviction_budget_or_throttle" {
			return ErrProtocol
		}
		switch p.State {
		case "pending", "gone", "terminating", "eviction_requested", "blocked", "skipped":
		default:
			return ErrProtocol
		}
		if len(p.Budgets) > drainLimit {
			return ErrProtocol
		}
		for _, b := range p.Budgets {
			if b.Name == "" || b.UID == "" || b.DisruptionsAllowed < 0 {
				return ErrProtocol
			}
		}
	}
	copy := *result.Drain
	drainPhase(&copy)
	if copy.Phase != result.Drain.Phase {
		return ErrProtocol
	}
	return nil
}
