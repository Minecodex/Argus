-- name: ListRunPendingActions :many
SELECT * FROM pending_actions
WHERE run_id=$1 AND enterprise_id=$2 AND creator_subject_id=$3 AND creator_subject_type='user'
ORDER BY created_at,id;

-- name: HasRunPendingActionEvent :one
SELECT EXISTS(SELECT 1 FROM conversation_events
WHERE run_id=$1 AND enterprise_id=$2 AND event_type='pending_action_created'
AND payload->>'action_ref'=sqlc.arg(action_ref)::text);
