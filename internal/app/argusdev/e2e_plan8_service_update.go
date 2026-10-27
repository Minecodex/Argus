package argusdev

import (
	"encoding/json"

	"github.com/google/uuid"
	api "github.com/kakj-go/Argus/internal/gen/openapi/monitoringapi"
	"github.com/kakj-go/Argus/internal/monitoring"
)

// Execution-only draft identities never belong in the public API request.
func plan8ServiceUpdateBody(row monitoring.Collection, collector uuid.UUID, enabled bool) map[string]any {
	var config any = row.Config
	var writable api.MonitoringCollectionConfigInput
	if json.Unmarshal(row.Config, &writable) == nil {
		config = writable
	}
	input := map[string]any{"name": row.Name, "integration": row.Integration, "target": row.Target, "collector_id": collector, "config": config, "enabled": enabled, "credential_update": "keep"}
	if row.ApplicationID != nil {
		input["application_id"] = *row.ApplicationID
	}
	if row.ServiceID != nil {
		input["service_id"] = *row.ServiceID
	}
	return map[string]any{"kind": "collection", "operation": "update", "id": row.ID, "expected_version": row.Version, "collection": input}
}

func plan8ServiceEnabledBody(row monitoring.Collection, enabled bool) map[string]any {
	return map[string]any{"kind": "collection", "operation": "set_enabled", "id": row.ID, "expected_version": row.Version, "collection": map[string]any{"enabled": enabled}}
}
