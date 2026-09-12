-- name: CreateBastionScope :one
INSERT INTO bastion_scopes (id, enterprise_id, name, environment, labels, labels_hash, onboarding_mode)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: BastionNameAvailable :one
-- A Bastion Scope and its Bastion Host share the user-visible
-- name. Both live-name namespaces must therefore be free before preview.
SELECT NOT EXISTS (
  SELECT 1 FROM bastion_scopes AS scope
  WHERE scope.enterprise_id = sqlc.arg('enterprise_id')
    AND lower(scope.name) = lower(sqlc.arg('name')) AND scope.status <> 'deleted'
) AND NOT EXISTS (
  SELECT 1 FROM hosts AS host
  WHERE host.enterprise_id = sqlc.arg('enterprise_id')
    AND lower(host.name) = lower(sqlc.arg('name')) AND host.status <> 'deleted'
) AS available;

-- name: AttachBastionRootHost :one
-- Creation is one transaction: link the preallocated root Host before the
-- newly created Scope can become externally visible.
UPDATE bastion_scopes SET connector_host_id = $3, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND status = 'pending' AND connector_host_id IS NULL
RETURNING *;

-- name: GetBastionScope :one
SELECT s.*,
  COALESCE((SELECT tunnel.status::text FROM connector_control_tunnels AS tunnel
   WHERE tunnel.bastion_scope_id = s.id
   ORDER BY tunnel.epoch DESC, tunnel.updated_at DESC LIMIT 1), ''::text)::text AS control_tunnel_status,
  (SELECT count(*) FROM hosts h WHERE h.bastion_scope_id = s.id AND h.role = 'managed_host' AND h.status <> 'deleted')::bigint AS member_count
  ,(SELECT operation.id FROM host_removal_operations operation
    WHERE operation.enterprise_id=s.enterprise_id AND operation.bastion_scope_id=s.id AND operation.target_type='bastion_scope'
      AND operation.removal_generation=s.removal_generation
      AND s.status IN ('draining','uninstalling','removal_failed','cleanup_unknown','uninstalled')
      AND ((s.active_connector_id IS NOT NULL AND operation.connector_id=s.active_connector_id)
        OR (s.active_connector_id IS NULL AND s.status='uninstalled' AND EXISTS (
          SELECT 1 FROM hosts root WHERE root.id=s.connector_host_id AND root.enterprise_id=s.enterprise_id
            AND root.role='bastion' AND root.status='uninstalled' AND root.removal_generation=s.removal_generation
            AND root.connector_id=operation.connector_id)))
    ORDER BY operation.created_at DESC,operation.id DESC LIMIT 1) AS removal_operation_id
FROM bastion_scopes s WHERE s.id = $1 AND s.enterprise_id = $2 AND s.status <> 'deleted';

-- name: ListBastionScopes :many
SELECT s.*,
  COALESCE((SELECT tunnel.status::text FROM connector_control_tunnels AS tunnel
   WHERE tunnel.bastion_scope_id = s.id
   ORDER BY tunnel.epoch DESC, tunnel.updated_at DESC LIMIT 1), ''::text)::text AS control_tunnel_status,
  (SELECT count(*) FROM hosts h WHERE h.bastion_scope_id = s.id AND h.role = 'managed_host' AND h.status <> 'deleted')::bigint AS member_count
  ,(SELECT operation.id FROM host_removal_operations operation
    WHERE operation.enterprise_id=s.enterprise_id AND operation.bastion_scope_id=s.id AND operation.target_type='bastion_scope'
      AND operation.removal_generation=s.removal_generation
      AND s.status IN ('draining','uninstalling','removal_failed','cleanup_unknown','uninstalled')
      AND ((s.active_connector_id IS NOT NULL AND operation.connector_id=s.active_connector_id)
        OR (s.active_connector_id IS NULL AND s.status='uninstalled' AND EXISTS (
          SELECT 1 FROM hosts root WHERE root.id=s.connector_host_id AND root.enterprise_id=s.enterprise_id
            AND root.role='bastion' AND root.status='uninstalled' AND root.removal_generation=s.removal_generation
            AND root.connector_id=operation.connector_id)))
    ORDER BY operation.created_at DESC,operation.id DESC LIMIT 1) AS removal_operation_id
FROM bastion_scopes s WHERE s.enterprise_id = $1 AND s.status <> 'deleted' ORDER BY s.created_at, s.id;

-- name: UpdateBastionScope :one
UPDATE bastion_scopes SET name = COALESCE(sqlc.narg('name'), name), environment = COALESCE(sqlc.narg('environment'), environment),
 labels = COALESCE(sqlc.narg('labels'), labels), labels_hash = COALESCE(sqlc.narg('labels_hash'), labels_hash),
 resource_version = resource_version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND resource_version = $3 AND status <> 'deleted' RETURNING *;

-- name: FenceBastionScope :one
UPDATE bastion_scopes SET fencing_generation = fencing_generation + 1, active_connector_id = NULL, status = 'offline', resource_version = resource_version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND resource_version = $3 AND status IN ('active','offline','uninstalled','suspected_offline') RETURNING *;

-- name: TouchPendingBastionScope :one
-- pending(尚未注册)作用域的替换:撤销旧令牌并递增版本,不发生 fencing。
UPDATE bastion_scopes SET resource_version = resource_version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 AND resource_version = $3 AND status = 'pending' RETURNING *;

-- name: DeleteBastionScope :one
UPDATE bastion_scopes AS scope SET status = 'deleted', deleted_at = now(), resource_version = scope.resource_version + 1, updated_at = now()
WHERE scope.id = $1 AND scope.enterprise_id = $2 AND scope.resource_version = $3
AND scope.status IN ('uninstalled','pending')
AND NOT EXISTS (SELECT 1 FROM hosts AS host WHERE host.bastion_scope_id = scope.id AND host.role = 'managed_host' AND host.status <> 'deleted')
AND NOT EXISTS (SELECT 1 FROM hosts AS host WHERE host.bastion_scope_id = scope.id AND host.role = 'bastion' AND host.status <> 'deleted')
AND NOT EXISTS (
  SELECT 1 FROM connector_commands AS command
  JOIN connectors AS connector ON connector.id = command.connector_id AND connector.enterprise_id = command.enterprise_id
  WHERE connector.bastion_scope_id = scope.id
    AND command.status IN ('queued','dispatched','acknowledged','running','delivery_unknown','result_unknown')
)
RETURNING *;

-- name: CreateConnectorEnrollmentToken :one
INSERT INTO connector_enrollment_tokens (id, preallocated_connector_id, enterprise_id, role, purpose, bastion_scope_id, kubernetes_cluster_id, preallocated_host_id, token_hash, policy, expires_at, created_by, release_version_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: GetEnrollmentTokenByHash :one
SELECT * FROM connector_enrollment_tokens WHERE token_hash = $1;

-- name: GetEnrollmentTokenForUpdate :one
SELECT * FROM connector_enrollment_tokens WHERE token_hash = $1 FOR UPDATE;

-- name: LockHostEnrollmentTokensForCancellation :many
SELECT * FROM connector_enrollment_tokens
WHERE enterprise_id=$1 AND preallocated_host_id=$2 ORDER BY created_at,id FOR UPDATE;

-- name: LockBastionEnrollmentTokensForCancellation :many
SELECT * FROM connector_enrollment_tokens
WHERE enterprise_id=$1 AND bastion_scope_id=$2 ORDER BY created_at,id FOR UPDATE;

-- name: ConsumeEnrollmentToken :one
UPDATE connector_enrollment_tokens SET status = 'consumed', consumed_at = now(), consumed_device_hash = $2, registered_connector_id = $3
WHERE id = $1 AND status = 'active' AND expires_at > now() RETURNING *;

-- name: RevokeActiveEnrollmentTokens :exec
UPDATE connector_enrollment_tokens SET status = 'revoked'
WHERE enterprise_id = $1 AND status = 'active' AND (bastion_scope_id = $2 OR kubernetes_cluster_id = $2);

-- name: CreateConnector :one
INSERT INTO connectors (id, enterprise_id, role, name, host_id, bastion_scope_id, kubernetes_cluster_id, instance_id, device_fingerprint_hash, public_key_hash, software_version, capabilities, certificate_expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: GetConnector :one
SELECT * FROM connectors WHERE id = $1 AND enterprise_id = $2;

-- name: GetConnectorByID :one
SELECT * FROM connectors WHERE id = $1;

-- name: ListConnectors :many
SELECT * FROM connectors WHERE enterprise_id = $1 ORDER BY created_at, id;

-- name: GetConnectorByInstance :one
SELECT * FROM connectors WHERE enterprise_id = $1 AND instance_id = $2;

-- name: GetLiveConnectorByInstanceForUpdate :one
SELECT * FROM connectors
WHERE enterprise_id=$1 AND instance_id=$2 AND status NOT IN ('revoked','uninstalled')
ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE;

-- name: ActivateBastionConnector :one
UPDATE bastion_scopes SET connector_host_id = $3, active_connector_id = $4, status = 'active', relay_status='pending', resource_version = resource_version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 RETURNING *;

-- name: SetBastionRelayStatus :execrows
UPDATE bastion_scopes SET relay_status=sqlc.arg('relay_status'), relay_error_code=sqlc.arg('relay_error_code'),
 relay_address=CASE WHEN sqlc.arg('relay_address')::text<>'' THEN sqlc.arg('relay_address') ELSE relay_address END,
 relay_https_port=CASE WHEN sqlc.arg('relay_port_generation')::bigint>0 THEN sqlc.arg('relay_https_port') ELSE relay_https_port END,
 relay_gateway_port=CASE WHEN sqlc.arg('relay_port_generation')::bigint>0 THEN sqlc.arg('relay_gateway_port') ELSE relay_gateway_port END,
 relay_port_generation=CASE WHEN sqlc.arg('relay_port_generation')::bigint>0 THEN sqlc.arg('relay_port_generation') ELSE relay_port_generation END,
 updated_at=now()
WHERE enterprise_id=sqlc.arg('enterprise_id') AND active_connector_id=sqlc.arg('active_connector_id')
  AND status IN ('active','suspected_offline','offline','draining','uninstalling','removal_failed','cleanup_unknown')
 AND (relay_port_generation=0 OR (
   relay_port_generation=sqlc.arg('relay_port_generation')
   AND relay_https_port=sqlc.arg('relay_https_port')
   AND relay_gateway_port=sqlc.arg('relay_gateway_port')
 ));

-- name: ActivateKubernetesConnector :one
UPDATE kubernetes_clusters SET connector_id = $3, connection_status = 'connected', resource_version = resource_version + 1, updated_at = now()
WHERE id = $1 AND enterprise_id = $2 RETURNING *;

-- name: RestoreHostConnectorOnline :execrows
UPDATE hosts SET connection_status='online',last_seen_at=now(),updated_at=now()
WHERE enterprise_id=$1 AND connector_id=$2 AND status IN ('active','disabled');

-- name: MarkHostConnectorOffline :execrows
UPDATE hosts SET connection_status='offline',updated_at=now()
WHERE enterprise_id=$1 AND connector_id=$2 AND status IN ('active','disabled','draining','uninstalling');

-- name: MarkBastionConnectorOfflineForTakeover :execrows
UPDATE bastion_scopes SET status='offline',relay_status='offline',updated_at=now()
WHERE enterprise_id=$1 AND active_connector_id=$2
  AND status IN ('active','suspected_offline','offline');

-- name: RestoreBastionConnectorOnline :execrows
UPDATE bastion_scopes SET status = 'active', resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND active_connector_id = $2 AND status IN ('suspected_offline','offline');

-- name: RestoreKubernetesConnectorOnline :execrows
UPDATE kubernetes_clusters SET connection_status = 'connected', resource_version = resource_version + 1, updated_at = now()
WHERE enterprise_id = $1 AND connector_id = $2 AND status <> 'deleted' AND connection_status IN ('degraded','disconnected');

-- name: RevokeActiveHostEnrollmentTokens :execrows
UPDATE connector_enrollment_tokens SET status='revoked'
WHERE enterprise_id=$1 AND preallocated_host_id=$2 AND status='active';

-- name: DeleteUnregisteredManagedHost :one
UPDATE hosts SET status='deleted',connection_status='offline',deleted_at=now(),resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND resource_version=$3 AND role='managed_host' AND status='active' AND connector_id IS NULL
RETURNING *;

-- name: DeleteUnregisteredBastionRootHost :one
UPDATE hosts SET status='deleted',connection_status='offline',deleted_at=now(),resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND bastion_scope_id=$3 AND role='bastion' AND status='active' AND connector_id IS NULL
RETURNING *;
