package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

// Deliberately excludes plans, command payloads/results, credential/lease
// references, private inputs and endpoint URLs. Capture before owned teardown.
func plan8ServiceOperationEvidenceSQL(enterprise uuid.UUID) string {
	return fmt.Sprintf(`SELECT json_build_object(
 'operations',(SELECT coalesce(json_agg(t),'[]') FROM (SELECT collector_id,operation,executor_kind,status,error_code,attempts,created_at,completed_at FROM telemetry_collector_operations WHERE enterprise_id='%[1]s' ORDER BY created_at DESC LIMIT 25) t),
 'commands',(SELECT coalesce(json_agg(t),'[]') FROM (SELECT connector_id,connection_epoch,command_type,status,error_code,payload->>'collector_id' AS collector_id,created_at,acknowledged_at,completed_at FROM connector_commands WHERE enterprise_id='%[1]s' AND command_type='collector_management' ORDER BY created_at DESC LIMIT 25) t),
 'tunnels',(SELECT coalesce(json_agg(t),'[]') FROM (SELECT collector_id,connector_id,status,epoch,fence,owner_connection_epoch,lease_expires_at,last_heartbeat_at FROM telemetry_tunnels WHERE enterprise_id='%[1]s' ORDER BY created_at DESC LIMIT 25) t),
 'source_counts',(SELECT coalesce(json_agg(t),'[]') FROM (SELECT collector_id,resource_type,source_type,config_revision,count(*) AS source_count FROM telemetry_sources WHERE enterprise_id='%[1]s' GROUP BY collector_id,resource_type,source_type,config_revision ORDER BY config_revision DESC LIMIT 50) t))`, enterprise)
}

func (a *App) plan8ServiceFailureEvidence(env *E2EEnvironment, row monitoring.Collection) {
	a.plan8ServiceReadinessEvidence(env, "apply-"+row.ID.String())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	proof := map[string]any{"collection": row}
	if enterprise, err := uuid.Parse(env.State.Values["enterprise_id"]); err == nil {
		state, err := a.postgresQuery(ctx, env, plan8ServiceOperationEvidenceSQL(enterprise))
		proof["state_query_succeeded"] = err == nil
		if err == nil {
			var facts any
			if json.Unmarshal([]byte(state), &facts) == nil {
				proof["operations"] = facts
			}
		}
	}
	raw, _ := json.MarshalIndent(redactJSON(proof), "", "  ")
	_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-service-apply-"+row.ID.String()+".json"), raw)
}
