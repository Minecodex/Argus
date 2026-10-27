-- name: CreateKubernetesHelmInput :exec
INSERT INTO kubernetes_helm_inputs(id,enterprise_id,operation_id,cluster_id,namespace_id,namespace_uid,actor_id,actor_type,authorization_version,request_hash,envelope)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: GetKubernetesHelmInput :one
SELECT * FROM kubernetes_helm_inputs WHERE enterprise_id=$1 AND id=$2;

-- name: BindKubernetesHelmInput :execrows
UPDATE kubernetes_helm_inputs SET status='bound' WHERE enterprise_id=$1 AND id=$2 AND operation_id=$3 AND status='staged' AND expires_at>now();

-- name: ResolveKubernetesHelmInput :execrows
UPDATE kubernetes_helm_inputs SET envelope=sqlc.arg(envelope)
WHERE enterprise_id=sqlc.arg(enterprise_id) AND id=sqlc.arg(id) AND operation_id=sqlc.arg(operation_id)
 AND request_hash=sqlc.arg(request_hash) AND status='staged' AND expires_at>now();

-- name: DeleteKubernetesHelmInput :exec
DELETE FROM kubernetes_helm_inputs WHERE enterprise_id=$1 AND id=$2;

-- name: DeleteRetiredKubernetesHelmInputs :exec
DELETE FROM kubernetes_helm_inputs AS input WHERE expires_at<=now() OR status='bound' AND EXISTS(SELECT 1 FROM kubernetes_operations o WHERE o.id=input.operation_id AND o.enterprise_id=input.enterprise_id AND o.status IN ('succeeded','failed','cancelled'));

-- name: AcquireKubernetesHelmLock :one
INSERT INTO kubernetes_helm_locks(enterprise_id,cluster_id,namespace_uid,release_name,operation_id) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(cluster_id,namespace_uid,release_name) DO UPDATE SET operation_id=EXCLUDED.operation_id,fence=kubernetes_helm_locks.fence+1,updated_at=now()
WHERE EXISTS(SELECT 1 FROM kubernetes_operations o WHERE o.id=kubernetes_helm_locks.operation_id AND o.enterprise_id=kubernetes_helm_locks.enterprise_id AND (o.status IN ('succeeded','failed','cancelled') OR o.status='result_unknown' AND o.started_at<now()-interval '6 minutes'))
RETURNING *;

-- name: GetKubernetesHelmLock :one
SELECT * FROM kubernetes_helm_locks WHERE enterprise_id=$1 AND cluster_id=$2 AND namespace_uid=$3 AND release_name=$4;
