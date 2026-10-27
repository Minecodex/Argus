-- name: ClaimKubernetesNamespaceSync :one
WITH candidate AS (
 SELECT c.id,c.enterprise_id,c.resource_version
 FROM kubernetes_clusters c LEFT JOIN kubernetes_namespace_sync_states s ON s.cluster_id=c.id
 WHERE c.status='active' AND (s.cluster_id IS NULL OR
   (s.last_attempt_at<now()-interval '1 minute' AND (s.lease_expires_at IS NULL OR s.lease_expires_at<now())))
 ORDER BY s.last_attempt_at ASC NULLS FIRST,c.id LIMIT 1 FOR UPDATE OF c SKIP LOCKED
)
INSERT INTO kubernetes_namespace_sync_states(cluster_id,enterprise_id,lease_token,lease_expires_at,last_attempt_at,snapshot_started_at,observed_cluster_version,status)
SELECT id,enterprise_id,$1,now()+interval '2 minutes',clock_timestamp(),clock_timestamp(),resource_version,'syncing' FROM candidate
ON CONFLICT(cluster_id) DO UPDATE SET lease_token=EXCLUDED.lease_token,lease_expires_at=EXCLUDED.lease_expires_at,
 last_attempt_at=EXCLUDED.last_attempt_at,snapshot_started_at=EXCLUDED.snapshot_started_at,
 observed_cluster_version=EXCLUDED.observed_cluster_version,status='syncing'
WHERE kubernetes_namespace_sync_states.lease_expires_at IS NULL OR kubernetes_namespace_sync_states.lease_expires_at<now()
RETURNING *;

-- name: LockNamespaceInventoryCluster :one
SELECT * FROM kubernetes_clusters WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: FinishKubernetesNamespaceSync :execrows
UPDATE kubernetes_namespace_sync_states SET lease_token=NULL,lease_expires_at=NULL,status=sqlc.arg(status),
 error_code=sqlc.arg(error_code),snapshot_resource_version=sqlc.arg(snapshot_resource_version),
 namespace_count=CASE WHEN sqlc.arg(status)::text='healthy' THEN sqlc.arg(namespace_count)::integer ELSE namespace_count END,
 last_success_at=CASE WHEN sqlc.arg(status)::text='healthy' THEN clock_timestamp() ELSE last_success_at END
WHERE cluster_id=sqlc.arg(cluster_id) AND enterprise_id=sqlc.arg(enterprise_id) AND lease_token=sqlc.arg(lease_token)
 AND lease_expires_at>now();

-- name: ListClusterNamespaceInventory :many
SELECT * FROM kubernetes_namespace_resources WHERE enterprise_id=$1 AND cluster_id=$2 ORDER BY id;

-- name: GetKubernetesNamespaceSyncState :one
SELECT * FROM kubernetes_namespace_sync_states WHERE enterprise_id=$1 AND cluster_id=$2;

-- name: RetireMissingKubernetesNamespaces :exec
UPDATE kubernetes_namespace_resources SET status='deleted',version=version+1,observed_at=clock_timestamp()
WHERE enterprise_id=sqlc.arg(enterprise_id) AND cluster_id=sqlc.arg(cluster_id) AND status='active'
 AND namespace_uid<>ALL(sqlc.arg(namespace_uids)::text[]) AND observed_at<=sqlc.arg(snapshot_started_at);
