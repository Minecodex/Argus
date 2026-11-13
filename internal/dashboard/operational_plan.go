package dashboard

import "github.com/kakj-go/Argus/internal/kubernetesworkbench"

// Plan the union once for the selected panels. A custom status/table panel may
// request a core kind which is absent from the default overview.
func inventoryPanelTypes(panel Panel) []string {
	if len(panel.DataSource.ResourceTypes) > 0 {
		return panel.DataSource.ResourceTypes
	}
	switch panel.Type {
	case "resource_summary":
		return []string{"node", "pod", "deployment", "statefulset", "daemonset", "job", "pvc", "event"}
	case "node_allocation":
		return []string{"node", "pod"}
	case "resource_events":
		return []string{"event"}
	default:
		return kubernetesworkbench.DefaultSnapshotTypes()
	}
}
func operationalPlan(spec Spec, panels []PanelExecution) *operationalRound {
	round := &operationalRound{}
	selected := map[string]bool{}
	for _, panel := range panels {
		selected[panel.ID] = true
	}
	seen := map[string]bool{}
	for _, panel := range spec.Panels {
		if !selected[panel.ID] || panel.DataSource == nil || panel.DataSource.Kind != "kubernetes_inventory" {
			continue
		}
		for _, kind := range inventoryPanelTypes(panel) {
			if !seen[kind] {
				seen[kind] = true
				round.types = append(round.types, kind)
			}
		}
	}
	return round
}
