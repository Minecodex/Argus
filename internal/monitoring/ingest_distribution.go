package monitoring

import (
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"slices"
)

// Factory names and upstream versions do not establish Argus identity behavior.
// Admission stays pinned to the verified distribution contract at every entry.
func sdkDistributionEligible(protocol, version, collectorVersion, schema, status string, components []string) bool {
	return SDKProtocolEligible(protocol) && version == configbundle.DistributionVersion && collectorVersion == configbundle.CollectorVersion && schema == configbundle.CatalogConfigSchemaVersion && status == "supported" && slices.Contains(components, protocol) && slices.Contains(components, "argus_application_auth") && slices.Contains(components, "argus_application_identity")
}
