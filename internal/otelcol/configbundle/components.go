package configbundle

import (
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"slices"
)

const (
	DistributionVersion        = "0.3.2-plan8"
	CollectorVersion           = monitoringcatalog.CollectorVersion
	CatalogConfigSchemaVersion = "argus.collector_config/v1"
)

// DistributionComponents lists configuration IDs accepted by the locked
// binary, including upstream's compatibility aliases for existing builtins.
// Runtime factory names are verified independently below and by binary tests.
func DistributionComponents(platform string) []string {
	result := []string{"nop", "otlp", "hostmetrics", "filelog", "prometheus", "skywalking", "jaeger", "batch", "memory_limiter", "resource", "filter", "transform", "argus_identity", "argus_application_auth", "argus_application_identity", "file_storage", "health_check"}
	switch platform {
	case "linux_amd64", "linux_arm64":
		result = append(result, "systemd", "journald", "kubeletstats", "k8s_cluster", "k8sattributes", "argus_gateway_identity", "argus_log_identity")
	case "windows_amd64":
		result = append(result, "windows_service", "windowseventlog")
	default:
		return nil
	}
	for _, entry := range monitoringcatalog.All() {
		if entry.Selection == "candidate" && entry.Family != "ingestion" && slices.Contains(entry.Platforms, platform) {
			result = append(result, entry.Component)
		}
	}
	slices.Sort(result)
	return result
}

func DistributionRuntimeComponents(platform string) []string {
	result := DistributionComponents(platform)
	if result == nil {
		return nil
	}
	aliases := map[string]string{"hostmetrics": "host_metrics", "filelog": "file_log", "kubeletstats": "kubelet_stats", "windowseventlog": "windows_event_log", "k8sattributes": "k8s_attributes"}
	for index, value := range result {
		if canonical, ok := aliases[value]; ok {
			result[index] = canonical
		}
	}
	// The OTLP receiver and gRPC exporter now have distinct factory names.
	result = append(result, "otlp_grpc")
	slices.Sort(result)
	return result
}
