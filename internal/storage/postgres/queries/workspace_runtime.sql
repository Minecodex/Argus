-- name: CreateWorkspaceSandboxSession :one
INSERT INTO sandbox_sessions (id,enterprise_id,workspace_id,tool_call_id,profile_id,profile_revision,upstream_session_id,status,expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'creating',$8) RETURNING *;

-- name: GetWorkspaceSandboxSession :one
SELECT * FROM sandbox_sessions WHERE tool_call_id=$1 AND enterprise_id=$2;

-- name: SetWorkspaceSandboxUpstream :one
UPDATE sandbox_sessions SET upstream_session_id=$3,status='running',started_at=now(),updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='creating' RETURNING *;

-- name: ListWorkspaceSandboxSessions :many
SELECT * FROM sandbox_sessions WHERE workspace_id=$1 AND enterprise_id=$2 AND status NOT IN ('terminated','failed');

-- name: ListDeletingWorkspaces :many
SELECT * FROM workspaces WHERE status='deleting' AND (lease_until IS NULL OR lease_until<now()) ORDER BY updated_at LIMIT $1;

-- name: GetWorkspaceForAdmission :one
SELECT * FROM workspaces WHERE id=$1 AND namespace=$2 AND fence_token=$3 AND lease_until>now() AND status='ready';

-- name: GetWorkspaceQuota :one
SELECT * FROM workspace_quotas WHERE enterprise_id=$1;

-- name: ExpireWorkspaceUploads :many
UPDATE workspace_uploads SET status='failed',error_code='WORKSPACE_UPLOAD_EXPIRED',updated_at=now()
WHERE expires_at<now() AND status IN ('pending','uploading') RETURNING *;

-- name: ClaimWorkspaceDeletion :one
UPDATE workspaces SET lease_owner=$3,lease_until=now()+$4::interval,fence_token=fence_token+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='deleting' AND (lease_until IS NULL OR lease_until<now()) RETURNING *;

-- name: ClaimWorkspaceProvision :one
UPDATE workspaces SET lease_owner=$3,lease_until=now()+$4::interval,fence_token=fence_token+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='provisioning' AND (lease_until IS NULL OR lease_until<now()) RETURNING *;

-- name: FinishWorkspaceDeletion :execrows
UPDATE workspaces SET status='deleted',deleted_at=now(),version=version+1,lease_owner=NULL,lease_until=NULL,
active_pod_name=NULL,active_sandbox_id=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='deleting' AND lease_owner=$3 AND fence_token=$4 AND lease_until>now();

-- name: FailDeletedWorkspaceUploads :exec
UPDATE workspace_uploads SET status='failed',error_code='WORKSPACE_DELETED',updated_at=now()
WHERE workspace_id=$1 AND enterprise_id=$2 AND status IN ('pending','uploading');

-- name: CancelConversationAgentRuns :many
UPDATE runs SET status='cancelled',stop_reason='workspace_deleted',version=version+1,updated_at=now()
WHERE conversation_id=$1 AND enterprise_id=$2 AND status NOT IN ('succeeded','failed','cancelled') RETURNING *;

-- name: MarkConversationDeleted :one
UPDATE conversations SET status='deleted',version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 RETURNING *;

-- name: MarkConversationWorkspacesDeleting :exec
UPDATE workspaces SET status='deleting',version=version+1,updated_at=now()
WHERE conversation_id=$1 AND enterprise_id=$2 AND status NOT IN ('deleted','deleting');

-- name: ListDeletedConversationsForCleanup :many
SELECT * FROM conversations c WHERE status='deleted' AND files_cleaned_at IS NULL
AND NOT EXISTS(SELECT 1 FROM workspaces w WHERE w.conversation_id=c.id AND w.status<>'deleted')
AND NOT EXISTS(SELECT 1 FROM dashboard_query_jobs j JOIN runtime_tasks t ON t.id=j.task_id
 WHERE j.conversation_id=c.id AND t.status IN ('leased','running') AND t.lease_until>now()) LIMIT $1;

-- name: FinishConversationFileCleanup :exec
UPDATE conversations SET files_cleaned_at=now() WHERE id=$1 AND status='deleted';

-- name: RegisterWorkspaceSource :one
UPDATE workspaces SET source_result_refs=ARRAY(SELECT DISTINCT unnest(array_append(source_result_refs,sqlc.arg('result_ref')::text)) ORDER BY 1)
WHERE id=$1 AND enterprise_id=$2 AND lease_owner=$3 AND fence_token=$4 AND lease_until>now() AND status='ready'
RETURNING *;
