package alerting

import "strings"

func reservedPolicyLabel(key string) bool {
	return strings.HasPrefix(key, "argus_") || key == "resource_id" || key == "policy_id" || key == "severity" || key == "metric" || key == "alert_health"
}

// Static policy metadata reaches routing/grouping/noise decisions, while real
// metric dimensions and trusted identity retain precedence. Reading a legacy
// snapshot cannot turn a reserved static label into trusted provenance.
func mergePolicyLabels(configured, observed map[string]string) map[string]string {
	result := make(map[string]string, len(configured)+len(observed))
	for key, value := range configured {
		if !reservedPolicyLabel(key) {
			result[key] = value
		}
	}
	for key, value := range observed {
		result[key] = value
	}
	return result
}
