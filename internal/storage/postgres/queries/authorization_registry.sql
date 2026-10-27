-- name: LockPermissionRegistry :exec
SELECT pg_advisory_xact_lock(hashtextextended('argus:permission-registry', 0));

-- name: CurrentPermissionRegistryRevision :one
SELECT COALESCE(MAX(registry_version), 0)::integer AS revision FROM permissions;

-- name: BumpBuiltinRoleRegistryVersion :one
UPDATE roles SET version=version+1, updated_at=now()
WHERE id=sqlc.arg(id) AND enterprise_id=sqlc.arg(enterprise_id)
  AND identity_key=sqlc.arg(identity_key)::text
  AND version=sqlc.arg(expected_version) AND builtin=true
RETURNING id;
