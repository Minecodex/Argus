-- name: GetOwnedRun :one
SELECT r.* FROM runs r JOIN conversations c ON c.id=r.conversation_id AND c.enterprise_id=r.enterprise_id
WHERE r.id=$1 AND r.enterprise_id=$2 AND r.actor_user_id=$3 AND c.owner_user_id=$3 AND c.status<>'deleted';

-- name: GetOwnedRunForUpdate :one
SELECT r.* FROM runs r JOIN conversations c ON c.id=r.conversation_id AND c.enterprise_id=r.enterprise_id
WHERE r.id=$1 AND r.enterprise_id=$2 AND r.actor_user_id=$3 AND c.owner_user_id=$3 AND c.status<>'deleted'
FOR UPDATE OF c,r;

-- name: HasPendingAgentTask :one
SELECT EXISTS(SELECT 1 FROM runtime_tasks WHERE run_id=$1 AND enterprise_id=$2 AND queue='agent' AND status='pending');

-- name: ListExpiredPendingActions :many
SELECT a.* FROM pending_actions a LEFT JOIN approval_requests request ON request.pending_action_id=a.id
WHERE a.status IN ('prepared','awaiting_confirmation','awaiting_approval','ready')
AND (a.expires_at<=now() OR (a.status='awaiting_approval' AND request.status='pending' AND request.expires_at<=now()))
ORDER BY a.expires_at,a.id LIMIT $1;

-- name: ExpirePendingAction :one
UPDATE pending_actions a SET status='expired',error_code='ACTION_INVALIDATED',updated_at=now()
WHERE a.id=$1 AND a.enterprise_id=$2 AND a.status IN ('prepared','awaiting_confirmation','awaiting_approval','ready')
AND (a.expires_at<=now() OR (a.status='awaiting_approval' AND EXISTS(
 SELECT 1 FROM approval_requests request WHERE request.pending_action_id=a.id AND request.status='pending' AND request.expires_at<=now())))
RETURNING a.*;

-- name: GetConversationWorkspaceContext :one
SELECT id,status FROM workspaces WHERE conversation_id=$1 AND enterprise_id=$2
ORDER BY (status<>'deleted') DESC,created_at DESC,id DESC LIMIT 1;
