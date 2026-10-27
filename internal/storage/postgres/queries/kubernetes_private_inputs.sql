-- name: CreateKubernetesPrivateInput :exec
INSERT INTO kubernetes_private_inputs(id,enterprise_id,operation_id,cluster_id,namespace_id,namespace_uid,actor_id,actor_type,authorization_version,mutation_hash,envelope,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now()+interval '15 minutes');

-- name: GetKubernetesPrivateInput :one
SELECT * FROM kubernetes_private_inputs WHERE id=$1 AND enterprise_id=$2 AND expires_at>now();

-- name: BindKubernetesPrivateInput :execrows
UPDATE kubernetes_private_inputs SET status='bound',expires_at=now()+interval '24 hours'
WHERE id=$1 AND enterprise_id=$2 AND operation_id=$3 AND actor_id=$4 AND status='draft' AND expires_at>now();

-- name: DeleteKubernetesPrivateInput :exec
DELETE FROM kubernetes_private_inputs WHERE id=$1 AND enterprise_id=$2 AND status='draft';

-- name: DeleteRetiredKubernetesPrivateInputs :exec
DELETE FROM kubernetes_private_inputs AS input WHERE expires_at<now()
OR EXISTS(SELECT 1 FROM kubernetes_operations operation WHERE operation.enterprise_id=input.enterprise_id AND operation.id=input.operation_id AND operation.status IN ('succeeded','failed','cancelled'));
