-- name: CreatePlanCommandAuthorization :one
INSERT INTO plan_command_authorizations(id,enterprise_id,conversation_id,owner_user_id,plan_id,sequence,block_id,spec,spec_hash)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: GetPlanCommandAuthorization :one
SELECT * FROM plan_command_authorizations WHERE id=$1 AND enterprise_id=$2 AND conversation_id=$3 AND owner_user_id=$4;

-- name: LockPlanCommandAuthorization :one
SELECT * FROM plan_command_authorizations WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: ListPlanCommandAuthorizations :many
SELECT * FROM plan_command_authorizations WHERE plan_id=$1 AND enterprise_id=$2 ORDER BY sequence;

-- name: ActivatePlanCommandAuthorizations :exec
UPDATE plan_command_authorizations SET implementation_run_id=$3 WHERE plan_id=$1 AND enterprise_id=$2 AND implementation_run_id IS NULL;

-- name: UsePlanCommandAuthorization :execrows
UPDATE plan_command_authorizations SET used_request_id=$3 WHERE id=$1 AND enterprise_id=$2 AND used_request_id IS NULL;

-- name: BindActionPlanCommand :exec
UPDATE pending_actions SET approved_plan_command_id=$3 WHERE id=$1 AND enterprise_id=$2;

-- name: ValidActionPlanCommand :one
SELECT EXISTS(SELECT 1 FROM plan_command_authorizations g
JOIN conversation_interactions p ON p.id=g.plan_id AND p.enterprise_id=g.enterprise_id
JOIN remote_execution_requests x ON x.id=g.used_request_id AND x.enterprise_id=g.enterprise_id
JOIN runs r ON r.id=g.implementation_run_id AND r.enterprise_id=g.enterprise_id
WHERE g.id=$1 AND g.enterprise_id=$2 AND g.implementation_run_id=$3 AND x.action_ref=$4
AND x.content_hash=$5 AND p.status='confirmed' AND p.implementation_run_id=r.id AND r.approved_plan_id=p.id
AND p.authorization_version=r.authorization_version AND p.owner_user_id=r.actor_user_id
AND r.status IN ('pending','running','waiting_input','waiting_approval','waiting_system'));
-- name: ResetUnusedRemoteSession :one
UPDATE remote_execution_sessions s SET generation=$3,context_revision=0,completed_sequence=0,status='new',known_directory='',runtime_account='',interpreter_version='',last_active_at=now()
WHERE s.id=$1 AND s.enterprise_id=$2 AND s.active_request_id IS NULL AND s.status IN ('lost','reclaimed','untrusted')
AND NOT EXISTS (SELECT 1 FROM remote_execution_steps t WHERE t.session_id=s.id AND t.started_at IS NOT NULL)
RETURNING s.*;
