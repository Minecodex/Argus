package telemetrybinding

import "github.com/google/uuid"

// CollectorServerName is shared by certificate issuance and SDK connection
// metadata. It is independent of infrastructure address authorization.
func CollectorServerName(id uuid.UUID) string {
	return "collector-" + id.String() + ".argus.telemetry"
}
