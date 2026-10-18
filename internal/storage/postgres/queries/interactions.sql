-- name: CreateConversationInteraction :one
INSERT INTO conversation_interactions(id,enterprise_id,conversation_id,owner_user_id,run_id,tool_call_id,kind,family_id,revision,content,content_hash,authorization_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: GetConversationInteraction :one
SELECT * FROM conversation_interactions WHERE id=$1 AND enterprise_id=$2 AND conversation_id=$3 AND owner_user_id=$4;

-- name: GetToolInteraction :one
SELECT * FROM conversation_interactions WHERE tool_call_id=$1 AND enterprise_id=$2;

-- name: LockConversationInteraction :one
SELECT * FROM conversation_interactions WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 FOR UPDATE;

-- name: ListConversationInteractions :many
SELECT i.* FROM conversation_interactions i WHERE i.enterprise_id=$1 AND i.conversation_id=$2 AND i.owner_user_id=$3
AND i.status<>'superseded'
AND (i.kind<>'plan' OR i.status<>'pending' OR NOT EXISTS(SELECT 1 FROM tool_calls t WHERE t.run_id=i.run_id AND t.status IN ('prepared','dispatched')))
ORDER BY i.created_at,i.id;

-- name: ResolveConversationInteraction :one
UPDATE conversation_interactions SET status=$4,response=$5,implementation_run_id=$6,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$3 AND status='pending' RETURNING *;

-- name: CancelRunInteractions :exec
UPDATE conversation_interactions SET status='cancelled',version=version+1,updated_at=now()
WHERE run_id=$1 AND enterprise_id=$2 AND status='pending';

-- name: LinkRunApprovedPlan :one
UPDATE runs SET approved_plan_id=$3 WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: AttachInteractionImplementation :one
UPDATE conversation_interactions SET implementation_run_id=$3 WHERE id=$1 AND enterprise_id=$2 AND status='confirmed' RETURNING *;

-- name: LatestRunPlan :one
SELECT * FROM conversation_interactions WHERE run_id=$1 AND enterprise_id=$2 AND kind='plan' ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: HasPendingRunInteraction :one
SELECT EXISTS(SELECT 1 FROM conversation_interactions WHERE run_id=$1 AND enterprise_id=$2 AND status='pending');

-- name: GetPendingRunInteraction :one
SELECT * FROM conversation_interactions WHERE run_id=$1 AND enterprise_id=$2 AND status='pending' LIMIT 1;

-- name: FreezeActionConfirmationPolicy :one
UPDATE pending_actions SET confirmation_mode=$3,confirmation_policy_version=$4 WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: SetConfirmationSource :exec
UPDATE user_confirmations SET source=$3 WHERE pending_action_id=$1 AND enterprise_id=$2;

-- name: CancelStoppedReadyAction :one
UPDATE pending_actions SET status='cancelled',error_code='TOOL_CANCELLED',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='ready' RETURNING *;

-- name: ListAnsweredQuestionTails :many
SELECT t.* FROM conversation_interactions i
JOIN tool_calls head ON head.id=i.tool_call_id AND head.enterprise_id=i.enterprise_id
JOIN tool_calls t ON t.model_call_id=head.model_call_id AND t.enterprise_id=i.enterprise_id AND t.call_sequence>head.call_sequence
WHERE i.run_id=$1 AND i.enterprise_id=$2 AND i.kind='user_input' AND i.status IN ('answered','skipped') AND t.status='prepared'
ORDER BY t.call_sequence;

-- name: HasUnfinishedStepTools :one
SELECT EXISTS(SELECT 1 FROM tool_calls WHERE step_id=$1 AND enterprise_id=$2 AND status IN ('prepared','dispatched'));
