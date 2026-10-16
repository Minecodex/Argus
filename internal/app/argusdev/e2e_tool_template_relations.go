package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// These pending Connector records only verify resource relationships; they do
// not claim a new enrollment, heartbeat or observed monitoring values.
func (a *App) templateConnectorFacts(ctx context.Context, env *E2EEnvironment) error {
	e, host, cluster := env.State.Values["enterprise_id"], env.State.Values["m7_host_id"], env.State.Values["m3_cluster_id"]
	for _, id := range []string{e, host, cluster} {
		if uuid.Validate(id) != nil {
			return fmt.Errorf("missing owned Connector relation identity")
		}
	}
	hostConnector, clusterConnector := uuid.NewString(), uuid.NewString()
	sql := fmt.Sprintf(`INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at) VALUES('%s','%s','host','Template host relationship','%s','%s',decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 hour');INSERT INTO connectors(id,enterprise_id,role,name,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at) VALUES('%s','%s','kubernetes','Template cluster relationship','%s','%s',decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 hour');`, hostConnector, e, host, hostConnector, clusterConnector, e, cluster, clusterConnector)
	// Explicitly constructed software metadata, not a claimed enrollment or heartbeat.
	sql += fmt.Sprintf(`UPDATE connectors SET software_version='fixture-v9.8.7' WHERE id='%s' AND enterprise_id='%s';`, hostConnector, e)
	if _, err := a.postgresQuery(ctx, env, sql); err != nil {
		return err
	}
	env.State.Values["template_host_connector_id"], env.State.Values["template_cluster_connector_id"] = hostConnector, clusterConnector
	proof, _ := json.MarshalIndent(map[string]any{"kind": "controlled-connector-relations", "connector_state": "pending metadata fixtures; no enrollment or telemetry claim", "host_id": host, "cluster_id": cluster, "host_connector_id": hostConnector, "cluster_connector_id": clusterConnector}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "tool-template-connector-facts.json"), proof)
}
