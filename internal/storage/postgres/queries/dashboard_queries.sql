-- name: CreateDashboardQueryJob :one
INSERT INTO dashboard_query_jobs(id,enterprise_id,conversation_id,owner_user_id,run_id,dashboard_id,revision_id,authorization_version,task_id,request_key,input_hash,frozen_plan)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: FindDashboardQueryJob :one
SELECT * FROM dashboard_query_jobs WHERE enterprise_id=$1 AND conversation_id=$2 AND owner_user_id=$3 AND request_key=$4;

-- name: GetDashboardQueryJob :one
SELECT * FROM dashboard_query_jobs WHERE id=$1 AND enterprise_id=$2;

-- name: LockDashboardQueryJob :one
SELECT * FROM dashboard_query_jobs WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: CreateDashboardQueryAttempt :one
INSERT INTO dashboard_query_attempts(id,enterprise_id,job_id,ordinal,status) VALUES($1,$2,$3,$4,'fetching') RETURNING *;

-- name: AbandonDashboardQueryAttempts :exec
UPDATE dashboard_query_attempts SET status='abandoned' WHERE job_id=$1 AND enterprise_id=$2 AND status='fetching';

-- name: SetDashboardQueryJobState :one
UPDATE dashboard_query_jobs SET status=$3,attempt_id=$4,error_code=$5,version=version+1,updated_at=now() WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: SealDashboardQueryAttempt :one
UPDATE dashboard_query_attempts SET status='materialized',sealed_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='fetching' RETURNING *;

-- name: SealDashboardQueryJob :one
UPDATE dashboard_query_jobs SET status='materialized',manifest=$3,version=version+1,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='fetching' AND attempt_id=$4 AND manifest IS NULL RETURNING *;

-- name: CreateDashboardQueryFile :one
INSERT INTO dashboard_query_files(id,enterprise_id,job_id,attempt_id,panel_id,target_id,file_kind,byte_size,content_hash,chunks) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetDashboardQueryFile :one
SELECT * FROM dashboard_query_files WHERE id=$1 AND enterprise_id=$2;

-- name: ListDashboardQueryFiles :many
SELECT * FROM dashboard_query_files WHERE attempt_id=$1 AND enterprise_id=$2 ORDER BY file_kind,panel_id,target_id;

-- name: GetDashboardQueryDelivery :one
SELECT d.* FROM dashboard_query_deliveries d JOIN workspace_files f ON f.id=d.workspace_file_id
WHERE d.file_id=$1 AND d.workspace_id=$2 AND d.enterprise_id=$3 AND f.deleted_at IS NULL;

-- name: CreateDashboardQueryDelivery :exec
INSERT INTO dashboard_query_deliveries(file_id,enterprise_id,workspace_id,workspace_file_id) VALUES($1,$2,$3,$4) ON CONFLICT(file_id,workspace_id) DO NOTHING;

-- name: GetWorkspaceFileByPath :one
SELECT * FROM workspace_files WHERE workspace_id=$1 AND enterprise_id=$2 AND path=$3 AND deleted_at IS NULL;

-- name: RetryDashboardQueryTask :execrows
UPDATE runtime_tasks SET status='pending',available_at=now(),max_attempts=attempt+5,last_error_code=NULL,
 lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND queue='dashboard_query' AND status='failed';

-- name: CancelConversationDashboardQueryJobs :exec
UPDATE dashboard_query_jobs SET status='cancelled',error_code='CONVERSATION_DELETED',version=version+1,updated_at=now()
WHERE conversation_id=$1 AND enterprise_id=$2 AND status NOT IN ('complete','partial','cancelled','failed');

-- name: CancelQueuedDashboardQueryTasks :exec
UPDATE runtime_tasks SET status='cancelled',updated_at=now() WHERE status='pending' AND id IN
(SELECT j.task_id FROM dashboard_query_jobs j WHERE j.conversation_id=$1 AND j.enterprise_id=$2 AND j.status='cancelled');
