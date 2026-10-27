package observationscope

import (
	"slices"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func boundedSourceKeys(base string, source db.ListObservationSourcesRow, resources []Resource) []string {
	if source.NamespaceUid != "" || source.ResourceType != "kubernetes_cluster" || !slices.Contains(source.Signals, "metrics") || source.SourceType != "k8s_cluster" && source.SourceType != "kubeletstats" {
		return []string{base}
	}
	result := []string{}
	for _, resource := range resources {
		if !slices.Contains(source.ScopeResourceIds, resource.ID) {
			continue
		}
		if resource.Kind != "kubernetes_namespace" {
			return []string{base}
		}
		if resource.NamespaceUID != "" {
			result = append(result, base+"@"+resource.NamespaceUID)
		}
	}
	return result
}
