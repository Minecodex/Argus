-- name: CreateConnectorCertificate :one
INSERT INTO connector_certificates (id, connector_id, enterprise_id, serial_number, issuer_generation, certificate_request_name, certificate_pem, ca_bundle_pem, not_before, not_after)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetActiveConnectorCertificate :one
SELECT * FROM connector_certificates
WHERE connector_id = $1 AND enterprise_id = $2 AND status IN ('active','overlap') AND not_after > now()
ORDER BY not_after DESC LIMIT 1;

-- name: GetValidConnectorCertificateBySerial :one
SELECT * FROM connector_certificates
WHERE connector_id = $1 AND enterprise_id = $2 AND serial_number = $3
  AND status IN ('active','overlap') AND not_before <= now() AND not_after > now();

-- name: RequestConnectorCertificateRotation :one
UPDATE connectors SET certificate_rotation_requested_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND version = $3
  AND status NOT IN ('uninstalled','revoked') RETURNING *;

-- name: StartConnectorUninstall :one
UPDATE connectors SET status = 'draining', version = version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND version = $3 AND connection_epoch = $4 AND status = 'online'
RETURNING *;

-- name: MarkBastionScopeUninstalling :execrows
UPDATE bastion_scopes SET status = 'uninstalling', resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND active_connector_id = $2 AND status = 'active';

-- name: MarkKubernetesConnectorUninstalling :execrows
UPDATE kubernetes_clusters SET connection_status = 'degraded', resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND connector_id = $2 AND status <> 'deleted';

-- name: FinalizeConnectorUninstall :one
UPDATE connectors SET status = 'uninstalled', version = version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND status IN ('draining','online','suspected_offline','offline')
RETURNING *;

-- name: FinalizeBastionConnectorUninstall :execrows
UPDATE bastion_scopes SET active_connector_id = NULL, status = 'uninstalled', fencing_generation = fencing_generation + 1,
 resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND active_connector_id = $2 AND status IN ('active','uninstalling','suspected_offline','offline');

-- name: FinalizeKubernetesConnectorUninstall :execrows
UPDATE kubernetes_clusters SET connector_id = NULL, connection_status = 'disconnected', resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND connector_id = $2 AND status <> 'deleted';

-- name: MarkConnectorCertificatesOverlap :exec
UPDATE connector_certificates
SET status = 'overlap', not_after = LEAST(not_after, now() + interval '15 minutes')
WHERE connector_id = $1 AND enterprise_id = $2 AND status = 'active';

-- name: CompleteConnectorCertificateRotation :one
UPDATE connectors SET public_key_hash = $3, certificate_expires_at = $4,
 certificate_rotation_requested_at = NULL, version = version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND status NOT IN ('uninstalled','revoked') RETURNING *;

-- name: RevokeConnectorCertificates :exec
UPDATE connector_certificates SET status = 'revoked', revoked_at = now()
WHERE connector_id = $1 AND enterprise_id = $2 AND status IN ('active','overlap');

-- name: FenceConnectorForReplacement :execrows
UPDATE connectors SET status = 'revoked', connection_epoch = connection_epoch + 1,
 version = version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND status <> 'revoked';

-- name: DeleteConnectorSessionsForReplacement :execrows
DELETE FROM connector_sessions WHERE connector_id = $1 AND enterprise_id = $2;

-- name: AdvanceConnectorEpoch :one
UPDATE connectors SET connection_epoch = connection_epoch + 1, status = 'online', connected_at = now(), last_heartbeat_at = now(), updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND status NOT IN ('uninstalled','revoked') RETURNING *;

-- name: UpsertConnectorSession :one
INSERT INTO connector_sessions (connector_id, enterprise_id, gateway_instance_id, connection_epoch, capabilities, connected_at, last_heartbeat_at)
VALUES ($1,$2,$3,$4,$5,now(),now())
ON CONFLICT (connector_id) DO UPDATE SET gateway_instance_id = EXCLUDED.gateway_instance_id, connection_epoch = EXCLUDED.connection_epoch,
 capabilities = EXCLUDED.capabilities, connected_at = now(), last_heartbeat_at = now(), draining = false
RETURNING *;

-- name: GetConnectorSession :one
SELECT * FROM connector_sessions
WHERE connector_id=$1 AND connection_epoch=$2 AND draining=false
  AND last_heartbeat_at > now() - interval '95 seconds';

-- name: HeartbeatConnectorSession :execrows
UPDATE connector_sessions SET last_heartbeat_at = now()
WHERE connector_id = $1 AND enterprise_id = $2 AND connection_epoch = $3;

-- name: CloseConnectorSession :execrows
DELETE FROM connector_sessions WHERE connector_id = $1 AND enterprise_id = $2 AND connection_epoch = $3;

-- name: MarkConnectorDisconnected :execrows
UPDATE connectors SET status = 'suspected_offline', updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND connection_epoch = $3 AND status IN ('online','draining');

-- name: MarkBastionScopeConnectorSuspectedOffline :execrows
UPDATE bastion_scopes SET status = 'suspected_offline',relay_status='degraded', updated_at = now()
WHERE enterprise_id = $1 AND active_connector_id = $2 AND status = 'active';

-- name: MarkStaleConnectorsOffline :execrows
UPDATE connectors SET status = 'offline', updated_at = now()
WHERE status = 'suspected_offline' AND last_heartbeat_at < now() - interval '5 minutes';

-- name: MarkStaleBastionScopesOffline :execrows
UPDATE bastion_scopes AS scope SET status = 'offline', relay_status='offline', updated_at = now()
WHERE scope.status = 'suspected_offline' AND EXISTS (
  SELECT 1 FROM connectors AS connector
  WHERE connector.id = scope.active_connector_id AND connector.enterprise_id = scope.enterprise_id AND connector.status = 'offline'
);

-- name: CreateConnectorCommand :one
INSERT INTO connector_commands (id, command_id, enterprise_id, connector_id, connection_epoch, operation_ref, credential_lease_id, command_type, payload_schema_version, payload, payload_hash, idempotency_key, expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: GetLatestConnectorCommandForOperation :one
SELECT * FROM connector_commands WHERE enterprise_id=$1 AND operation_ref=$2 ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: GetConnectorCommand :one
SELECT * FROM connector_commands WHERE command_id = $1 AND connector_id = $2 AND connection_epoch = $3;

-- name: GetConnectorCommandByID :one
SELECT * FROM connector_commands WHERE id = $1;

-- name: ListUncertainConnectorCommands :many
SELECT * FROM connector_commands WHERE connector_id = $1 AND status IN ('delivery_unknown','result_unknown')
ORDER BY created_at LIMIT $2;

-- name: ListDispatchableConnectorCommands :many
SELECT command.* FROM connector_commands command
WHERE command.connector_id = $1 AND command.connection_epoch = $2 AND command.status = 'queued' AND command.expires_at > now()
AND (
  command.command_type <> 'collector_management' OR
  command.payload->>'transport' = 'direct' OR
  EXISTS (
    SELECT 1 FROM telemetry_tunnels tunnel
    WHERE tunnel.enterprise_id = command.enterprise_id
      AND tunnel.collector_id = (command.payload->>'collector_id')::uuid
      AND tunnel.transport = command.payload->>'transport'
      AND tunnel.status = 'established'
  )
)
AND (
  command.command_type <> 'collector_management' OR
  command.payload->>'route_kind' <> 'bastion_gateway' OR
  EXISTS (
    SELECT 1 FROM collector_instances gateway
    WHERE gateway.enterprise_id = command.enterprise_id
      AND gateway.id = (command.payload->>'gateway_collector_id')::uuid
      AND gateway.role = 'edge_gateway'
      AND gateway.status = 'converged'
  )
)
ORDER BY command.created_at LIMIT $3 FOR UPDATE SKIP LOCKED;

-- name: ExpireQueuedConnectorCommands :many
UPDATE connector_commands SET status = 'expired', error_code = 'CONNECTOR_COMMAND_EXPIRED', completed_at = now(), updated_at = now()
WHERE status = 'queued' AND expires_at <= now() RETURNING *;

-- name: TimeoutActiveConnectorCommands :many
UPDATE connector_commands SET status = 'timed_out', error_code = 'CONNECTOR_COMMAND_TIMED_OUT', completed_at = now(), updated_at = now()
WHERE status IN ('dispatched','acknowledged','running') AND expires_at <= now() RETURNING *;

-- name: TransitionConnectorCommand :one
UPDATE connector_commands SET status = $4, result = COALESCE(sqlc.narg('result'), result), result_hash = COALESCE(sqlc.narg('result_hash'), result_hash),
 error_code = COALESCE(sqlc.narg('error_code'), error_code), updated_at = now(),
 acknowledged_at = CASE WHEN $4 = 'acknowledged' THEN now() ELSE acknowledged_at END,
 started_at = CASE WHEN $4 = 'running' THEN now() ELSE started_at END,
 completed_at = CASE WHEN $4 IN ('succeeded','failed','timed_out','expired') THEN now() ELSE completed_at END
WHERE command_id = $1 AND connector_id = $2 AND connection_epoch = $3
  AND status = sqlc.arg('expected_status') RETURNING *;
