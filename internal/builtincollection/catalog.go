// Package builtincollection describes fixed collections owned by an existing
// Host or Cluster. These definitions do not create monitoring targets.
package builtincollection

import (
	"slices"

	"github.com/google/uuid"
)

const Version = "argus.builtin_collection/v1"

type Definition struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	ResourceTypes []string `json:"resource_types"`
	Platforms     []string `json:"platforms"`
	Component     string   `json:"component"`
	SourceType    string   `json:"source_type"`
	Placement     string   `json:"placement"`
	LegacyProfile string   `json:"legacy_profile"`
	ComponentTLS  bool     `json:"component_tls_supported"`
}

var definitions = []Definition{
	{"host_metrics", "Host metrics", []string{"host"}, []string{"linux_amd64", "linux_arm64", "windows_amd64"}, "hostmetrics", "hostmetrics", "host", "host-basic", false},
	{"host_process_metrics", "Host processes", []string{"host"}, []string{"linux_amd64", "linux_arm64", "windows_amd64"}, "hostmetrics", "host_processes", "host", "", false},
	{"host_systemd_metrics", "Linux system services", []string{"host"}, []string{"linux_amd64", "linux_arm64"}, "systemd", "host_systemd", "host", "", false},
	{"host_windows_service_metrics", "Windows system services", []string{"host"}, []string{"windows_amd64"}, "windows_service", "host_windows_services", "host", "", false},
	{"collector_metrics", "Collector metrics", []string{"host", "kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64", "windows_amd64"}, "prometheus", "collector_self", "all", "collector-self", false},
	{"kubernetes_cluster_metrics", "Cluster and workload state", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "k8s_cluster", "k8s_cluster", "kubernetes_gateway", "k8s-cluster", false},
	{"kubernetes_node_metrics", "Node and container metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "kubeletstats", "kubeletstats", "kubernetes_agent", "k8s-node-container", false},
	{"kubernetes_apiserver_metrics", "API server metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "prometheus", "kubernetes_apiserver", "kubernetes_gateway", "", false},
	{"kubernetes_coredns_metrics", "CoreDNS metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "prometheus", "kubernetes_coredns", "kubernetes_gateway", "", false},
	{"kubernetes_scheduler_metrics", "Scheduler metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "prometheus", "kubernetes_scheduler", "kubernetes_gateway", "", true},
	{"kubernetes_controller_metrics", "Controller Manager metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "prometheus", "kubernetes_controller_manager", "kubernetes_gateway", "", true},
	{"kubernetes_etcd_metrics", "etcd component metrics", []string{"kubernetes_cluster"}, []string{"linux_amd64", "linux_arm64"}, "prometheus", "kubernetes_etcd", "kubernetes_gateway", "", false},
}

func All() []Definition {
	out := slices.Clone(definitions)
	for i := range out {
		out[i].ResourceTypes = slices.Clone(out[i].ResourceTypes)
		out[i].Platforms = slices.Clone(out[i].Platforms)
	}
	return out
}
func Lookup(key string) (Definition, bool) {
	for _, entry := range All() {
		if entry.Key == key {
			return entry, true
		}
	}
	return Definition{}, false
}

// ScrapedSource identifies our fixed component sources, never SDK metrics or
// arbitrary Prometheus inputs whose metric named "up" may be business data.
func ScrapedSource(source string) bool {
	for _, definition := range definitions {
		if definition.Component == "prometheus" && definition.SourceType == source {
			return true
		}
	}
	return false
}
func StableID(resourceType string, resource uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(resource, []byte(Version+"/"+resourceType+"/"+key))
}

func ComponentsAvailable(definition Definition, components []string) bool {
	if !slices.Contains(components, definition.Component) {
		return false
	}
	return definition.Key != "kubernetes_cluster_metrics" && definition.Key != "kubernetes_node_metrics" || slices.Contains(components, "k8sattributes")
}
