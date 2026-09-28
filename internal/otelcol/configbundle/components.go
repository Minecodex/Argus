package configbundle

import "slices"

const (
	DistributionVersion        = "0.1.0-m7"
	CollectorVersion           = "0.133.0"
	CatalogConfigSchemaVersion = "argus.collector_config/v1"
)

// DistributionComponents uses runtime component IDs, not Go module names or
// collection profile IDs. Contract tests bind this inventory to each OCB file.
func DistributionComponents(platform string) []string {
	result := []string{"otlp", "hostmetrics", "filelog", "prometheus", "skywalking", "jaeger", "batch", "memory_limiter", "resource", "argus_identity", "file_storage", "health_check"}
	switch platform {
	case "linux_amd64", "linux_arm64":
		result = append(result, "journald", "kubeletstats", "k8s_cluster", "argus_gateway_identity")
	case "windows_amd64":
		result = append(result, "windowseventlog")
	default:
		return nil
	}
	slices.Sort(result)
	return result
}
