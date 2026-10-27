package queryengine

import "github.com/kakj-go/Argus/internal/telemetry/sourcekey"

// A scope qualification, not a claim that rows were lost or budget-truncated.
const NamespaceMetricQualification = "NAMESPACE_VERIFIED_OBSERVATIONS_ONLY: Namespace metrics include only observations with verified Namespace UID attribution; unverified observations are visible only with Cluster authorization."

func namespaceFacetedScope(keys []string) bool {
	for _, key := range keys {
		if _, _, uid, valid := sourcekey.Parse(key); valid && uid != "" {
			return true
		}
	}
	return false
}
