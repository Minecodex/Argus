-- name: LockConnectorCommandForReconciliation :one
SELECT * FROM connector_commands
WHERE command_id=$1 AND enterprise_id=$2 AND connector_id=$3 AND connection_epoch<=$4
FOR UPDATE;

-- name: MarkPreviousCollectorCommandsUncertain :many
UPDATE connector_commands SET status='result_unknown',error_code='CONNECTOR_DISCONNECTED',updated_at=now()
WHERE connector_id=$1 AND enterprise_id=$2 AND connection_epoch<$3
 AND command_type='collector_management' AND status IN ('dispatched','acknowledged','running')
RETURNING *;

-- name: ListUncertainCollectorCommands :many
SELECT * FROM connector_commands WHERE connector_id=$1 AND enterprise_id=$2
 AND command_type='collector_management' AND status IN ('delivery_unknown','result_unknown')
ORDER BY created_at DESC,id DESC LIMIT $3;
