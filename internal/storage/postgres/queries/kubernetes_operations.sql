-- name: CreateKubernetesOperation :one
INSERT INTO kubernetes_operations(id,enterprise_id,cluster_id,namespace_id,namespace_uid,actor_id,actor_type,authorization_version,cluster_version,pending_action_id,status,operation,type_key,target_name,target_uid,expected_resource_version,private_plan)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'awaiting_confirmation',$11,$12,$13,$14,$15,$16) RETURNING *;

-- name: GetKubernetesOperation :one
SELECT * FROM kubernetes_operations WHERE enterprise_id=$1 AND id=$2;

-- name: LockKubernetesOperation :one
SELECT * FROM kubernetes_operations WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: QueueKubernetesOperation :execrows
UPDATE kubernetes_operations SET status='queued',version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status='awaiting_confirmation';

-- name: StartKubernetesOperation :one
UPDATE kubernetes_operations SET status='running',started_at=now(),heartbeat_at=now(),version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status='queued' RETURNING *;

-- name: ObserveKubernetesOperation :execrows
UPDATE kubernetes_operations SET status='observing',result=$3,heartbeat_at=now(),version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status='running';

-- name: FinishKubernetesOperation :execrows
UPDATE kubernetes_operations SET status=$3,result=$4,error_code=$5,finished_at=now(),heartbeat_at=now(),version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status IN ('queued','running','observing','result_unknown');

-- name: UpdateKubernetesDrainProgress :execrows
UPDATE kubernetes_operations SET status='observing',result=$3,error_code='',heartbeat_at=now(),updated_at=now(),version=version+1
WHERE enterprise_id=$1 AND id=$2 AND operation='drain' AND status IN ('observing','result_unknown');

-- name: CancelKubernetesDrain :one
UPDATE kubernetes_operations SET status='cancelled',finished_at=now(),heartbeat_at=now(),updated_at=now(),version=version+1
WHERE enterprise_id=$1 AND id=$2 AND version=$3 AND operation='drain' AND status IN ('queued','running','observing','result_unknown') RETURNING *;

-- name: HeartbeatKubernetesOperation :exec
UPDATE kubernetes_operations SET heartbeat_at=now() WHERE enterprise_id=$1 AND id=$2 AND status IN ('running','observing');

-- name: ListKubernetesOperationsToRecover :many
SELECT * FROM kubernetes_operations WHERE status IN ('queued','running','observing','result_unknown') AND updated_at<now()-interval '30 seconds' AND (heartbeat_at IS NULL OR heartbeat_at<now()-interval '30 seconds')
ORDER BY heartbeat_at NULLS FIRST,id LIMIT $1;

-- name: ClaimKubernetesObservation :one
UPDATE kubernetes_operations SET heartbeat_at=now(),updated_at=now(),version=version+1,
status=CASE WHEN status='running' THEN 'result_unknown' ELSE status END
WHERE enterprise_id=$1 AND id=$2 AND version=$3 AND status IN ('running','observing','result_unknown')
AND (heartbeat_at IS NULL OR heartbeat_at<now()-interval '30 seconds') RETURNING *;

-- name: MarkKubernetesOperationUnknown :execrows
UPDATE kubernetes_operations SET status='result_unknown',error_code=$3,heartbeat_at=now(),updated_at=now(),version=version+1
WHERE enterprise_id=$1 AND id=$2 AND status='running';

-- name: MarkExecutionKubernetesResultUnknown :one
UPDATE executions SET status='result_unknown',kubernetes_operation_id=$3,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: ListKubernetesOperations :many
SELECT * FROM kubernetes_operations WHERE enterprise_id=$1 AND cluster_id=$2 AND id>sqlc.arg(after_id)::uuid
AND (sqlc.arg(cluster_wide)::boolean OR namespace_id=ANY(sqlc.arg(namespace_ids)::uuid[]))
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: BindKubernetesOperationCommand :execrows
UPDATE kubernetes_operations SET connector_command_id=$3 WHERE enterprise_id=$1 AND id=$2 AND status='running' AND connector_command_id IS NULL;

-- name: BindKubernetesDrainCommand :execrows
UPDATE kubernetes_operations SET connector_command_id=$3 WHERE enterprise_id=$1 AND id=$2 AND operation='drain' AND status IN ('observing','result_unknown');

-- name: RetireUnconfirmedKubernetesOperations :exec
UPDATE kubernetes_operations AS operation SET status=CASE WHEN action.status='failed' THEN 'failed' ELSE 'cancelled' END,
error_code='PENDING_ACTION_'||upper(action.status),finished_at=now(),updated_at=now(),version=operation.version+1
FROM pending_actions AS action WHERE action.id=operation.pending_action_id AND action.enterprise_id=operation.enterprise_id
AND operation.status='awaiting_confirmation' AND action.status IN ('cancelled','expired','rejected','invalidated','failed');
