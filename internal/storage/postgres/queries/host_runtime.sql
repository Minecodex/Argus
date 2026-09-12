-- name: UpsertHostRuntimeObservation :one
INSERT INTO host_runtime_observations (host_id,enterprise_id,connector_id,platform,openssh_status,rdp_status,rdp_nla_enabled,rdp_firewall_enabled,rdp_service_running,observed_at)
SELECT connector.host_id,connector.enterprise_id,connector.id,$3,$4,$5,$6,$7,$8,$9
FROM connectors connector
JOIN hosts host ON host.id=connector.host_id AND host.enterprise_id=connector.enterprise_id AND host.status<>'deleted'
WHERE connector.id=$1 AND connector.enterprise_id=$2 AND connector.role IN ('host','bastion') AND host.platform=$3
ON CONFLICT (host_id) DO UPDATE SET connector_id=EXCLUDED.connector_id,platform=EXCLUDED.platform,openssh_status=EXCLUDED.openssh_status,
 rdp_status=EXCLUDED.rdp_status,rdp_nla_enabled=EXCLUDED.rdp_nla_enabled,rdp_firewall_enabled=EXCLUDED.rdp_firewall_enabled,
 rdp_service_running=EXCLUDED.rdp_service_running,observed_at=EXCLUDED.observed_at,updated_at=now()
RETURNING *;

-- name: ListHostRuntimeObservations :many
SELECT * FROM host_runtime_observations WHERE enterprise_id=$1 AND host_id=ANY($2::uuid[]) ORDER BY host_id;

-- name: GetHostRuntimeObservation :one
SELECT * FROM host_runtime_observations WHERE enterprise_id=$1 AND host_id=$2;
