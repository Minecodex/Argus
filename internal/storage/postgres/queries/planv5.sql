-- name: GetMCPConnection :one
SELECT * FROM mcp_connections WHERE id = $1 AND enterprise_id = $2;

-- name: ListMCPConnections :many
SELECT * FROM mcp_connections WHERE enterprise_id = $1 ORDER BY name, id;

-- name: ListGrantedMCPConnections :many
SELECT c.* FROM mcp_connections c
JOIN mcp_connection_grants g ON g.connection_id = c.id AND g.enterprise_id = c.enterprise_id
WHERE c.enterprise_id = $1 AND g.user_id = $2
ORDER BY c.name, c.id;

-- name: CreateMCPConnection :one
INSERT INTO mcp_connections (id, enterprise_id, name, created_by)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: UpdateMCPConnection :one
UPDATE mcp_connections SET name=$3, status=$4, current_revision=$5,
    current_tool_snapshot_id=$6, health_status=$7, last_error_code=$8,
    authorization_epoch=authorization_epoch+1, version=version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$9 RETURNING *;

-- name: CreateMCPConnectionRevision :one
INSERT INTO mcp_connection_revisions (connection_id,enterprise_id,revision,endpoint,auth_type,credential_id,credential_version)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: GetMCPConnectionRevision :one
SELECT * FROM mcp_connection_revisions WHERE connection_id=$1 AND enterprise_id=$2 AND revision=$3;

-- name: CreateMCPToolSnapshot :one
INSERT INTO mcp_tool_snapshots (id,connection_id,enterprise_id,connection_revision,schema_hash,tools)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (connection_id,connection_revision,schema_hash) DO UPDATE SET schema_hash=EXCLUDED.schema_hash
RETURNING *;

-- name: GetMCPToolSnapshot :one
SELECT * FROM mcp_tool_snapshots WHERE id=$1 AND enterprise_id=$2;

-- name: ListMCPConnectionMembers :many
SELECT user_id FROM mcp_connection_grants WHERE connection_id=$1 AND enterprise_id=$2 ORDER BY user_id;

-- name: DeleteMCPConnectionGrants :exec
DELETE FROM mcp_connection_grants WHERE connection_id=$1 AND enterprise_id=$2;

-- name: GrantMCPConnection :exec
INSERT INTO mcp_connection_grants (connection_id,enterprise_id,user_id)
VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: HasMCPConnectionGrant :one
SELECT EXISTS(SELECT 1 FROM mcp_connection_grants g
JOIN enterprise_users u ON u.id=g.user_id AND u.enterprise_id=g.enterprise_id AND u.status='active'
JOIN enterprises e ON e.id=g.enterprise_id AND e.status='active'
WHERE g.connection_id=$1 AND g.enterprise_id=$2 AND g.user_id=$3)::boolean;

-- name: BumpMCPAuthorization :one
UPDATE mcp_connections SET authorization_epoch=authorization_epoch+1,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$3 RETURNING *;

-- name: GetConversationMCPConnections :many
SELECT connection_id FROM conversation_mcp_connections WHERE conversation_id=$1 AND enterprise_id=$2 ORDER BY connection_id;

-- name: ClearConversationMCPConnections :exec
DELETE FROM conversation_mcp_connections WHERE conversation_id=$1 AND enterprise_id=$2;

-- name: SelectConversationMCPConnection :exec
INSERT INTO conversation_mcp_connections (conversation_id,enterprise_id,connection_id)
VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: GetConversationWorkspace :one
SELECT * FROM workspaces WHERE conversation_id=$1 AND enterprise_id=$2 AND status<>'deleted';

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id=$1 AND enterprise_id=$2;

-- name: LockWorkspace :one
SELECT * FROM workspaces WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: CreateWorkspace :one
INSERT INTO workspaces (id,enterprise_id,conversation_id,pvc_name,namespace,capacity_bytes,environment_version)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: EnsureWorkspaceQuota :exec
INSERT INTO workspace_quotas (enterprise_id,limit_bytes) VALUES ($1,$2) ON CONFLICT DO NOTHING;

-- name: ReserveWorkspaceCapacity :one
UPDATE workspace_quotas SET reserved_bytes=reserved_bytes+$2, updated_at=now()
WHERE enterprise_id=$1 AND reserved_bytes+$2<=limit_bytes RETURNING *;

-- name: ReleaseWorkspaceCapacity :exec
UPDATE workspace_quotas SET reserved_bytes=GREATEST(0,reserved_bytes-$2),updated_at=now() WHERE enterprise_id=$1;

-- name: SetWorkspaceStatus :one
UPDATE workspaces SET status=$3,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$4 RETURNING *;

-- name: ClaimWorkspaceLease :one
UPDATE workspaces SET lease_owner=$3,lease_until=now()+$4::interval,fence_token=fence_token+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='ready'
AND fence_token=sqlc.arg(expected_fence)
AND (lease_until IS NULL OR lease_until<now()) RETURNING *;

-- name: RenewWorkspaceLease :execrows
UPDATE workspaces SET lease_until=now()+$5::interval,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND lease_owner=$3 AND fence_token=$4 AND lease_until>now();

-- name: ReleaseWorkspaceLease :execrows
UPDATE workspaces SET lease_owner=NULL,lease_until=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND lease_owner=$3 AND fence_token=$4;

-- name: SetWorkspaceAttachment :execrows
UPDATE workspaces SET active_pod_name=$5,active_sandbox_id=$6,last_used_at=now(),updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND lease_owner=$3 AND fence_token=$4 AND lease_until>now();

-- name: ListIdleWorkspaces :many
SELECT * FROM workspaces WHERE status='ready' AND active_pod_name IS NOT NULL
AND last_used_at<$1 AND (lease_until IS NULL OR lease_until<now()) ORDER BY last_used_at LIMIT $2;

-- name: CreateWorkspaceUpload :one
INSERT INTO workspace_uploads (id,enterprise_id,workspace_id,owner_user_id,name,path,expected_bytes,request_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: GetWorkspaceUpload :one
SELECT * FROM workspace_uploads WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3;

-- name: ClaimWorkspaceUpload :one
UPDATE workspace_uploads SET status='uploading',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 AND status='pending' AND expires_at>now() RETURNING *;

-- name: FinishWorkspaceUpload :execrows
UPDATE workspace_uploads SET status=$3,file_id=$4,error_code=$5,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='uploading';

-- name: CreateWorkspaceFile :one
INSERT INTO workspace_files (id,enterprise_id,workspace_id,conversation_id,name,path,byte_size,content_hash,media_type,source_result_refs)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetWorkspaceFile :one
SELECT * FROM workspace_files WHERE id=$1 AND enterprise_id=$2 AND deleted_at IS NULL;

-- name: ListWorkspaceFiles :many
SELECT * FROM workspace_files WHERE workspace_id=$1 AND enterprise_id=$2 AND deleted_at IS NULL ORDER BY created_at,id;

-- name: DeleteWorkspaceFileRecords :exec
UPDATE workspace_files SET deleted_at=now() WHERE workspace_id=$1 AND enterprise_id=$2 AND deleted_at IS NULL;

-- name: CreateFileDelivery :one
INSERT INTO file_deliveries (id,enterprise_id,conversation_id,workspace_id,run_id,name,media_type,byte_size,content_hash,object_key,source_result_refs)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;

-- name: GetFileDelivery :one
SELECT * FROM file_deliveries WHERE id=$1 AND enterprise_id=$2 AND deleted_at IS NULL;

-- name: ListWorkspaceDeliveries :many
SELECT * FROM file_deliveries WHERE workspace_id=$1 AND enterprise_id=$2 AND deleted_at IS NULL ORDER BY created_at,id;

-- name: DeleteWorkspaceDeliveryRecords :exec
UPDATE file_deliveries SET deleted_at=now() WHERE workspace_id=$1 AND enterprise_id=$2 AND deleted_at IS NULL;

-- name: CreateToolPresentation :one
INSERT INTO tool_presentations (id,enterprise_id,conversation_id,tool_call_id,tool_version,authorization_scope,template_hash,detail_data,resource_refs,status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: CreateTemplateAsset :exec
INSERT INTO template_assets (hash,source,runtime) VALUES($1,$2,'argus-template/v1') ON CONFLICT DO NOTHING;

-- name: GetToolPresentation :one
SELECT p.*,a.source AS template_source FROM tool_presentations p JOIN template_assets a ON a.hash=p.template_hash
WHERE p.tool_call_id=$1 AND p.enterprise_id=$2 AND p.conversation_id=$3;

-- name: SetRunToolSnapshot :one
UPDATE runs SET tool_snapshot=$3, tool_snapshot_hash=$4, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND tool_snapshot_hash='' RETURNING *;

-- name: ListConversationContextEvents :many
SELECT * FROM conversation_events WHERE conversation_id=$1 AND enterprise_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4;

-- name: GetModelToolCall :one
SELECT * FROM tool_calls WHERE step_id=$1 AND call_id=$2 AND enterprise_id=$3;

-- name: MarkToolDispatched :execrows
UPDATE tool_calls SET status='dispatched',dispatched_at=now(),updated_at=now()
WHERE tool_calls.id=$1 AND tool_calls.enterprise_id=$2 AND tool_calls.status='prepared'
AND EXISTS(SELECT 1 FROM runs r WHERE r.id=tool_calls.run_id AND r.enterprise_id=tool_calls.enterprise_id
  AND r.version=sqlc.arg(expected_run_version) AND r.status NOT IN ('succeeded','failed','cancelled'));

-- name: GetPersistedToolResult :one
SELECT * FROM tool_results WHERE tool_call_id=$1 AND enterprise_id=$2;

-- name: ListUnfinishedRunToolCalls :many
SELECT call.* FROM tool_calls call JOIN run_steps step ON step.id=call.step_id
WHERE call.run_id=$1 AND call.enterprise_id=$2 AND call.status IN ('prepared','dispatched')
ORDER BY step.sequence,call.call_sequence;

-- name: CreateModelToolCall :one
INSERT INTO tool_calls (id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status,source,model_call_id,call_sequence,connection_id,schema_hash,authorization_scope)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'prepared',$9,$10,$11,$12,$13,$14) RETURNING *;

-- name: GetActiveConversationSnapshot :one
SELECT * FROM context_snapshots WHERE conversation_id=$1 AND enterprise_id=$2 AND status='active';

-- name: MarkRunVerificationOnly :exec
UPDATE runs SET verification_only=true,updated_at=now() WHERE id=$1 AND enterprise_id=$2;

-- name: GetRunForUpdate :one
SELECT * FROM runs WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: GetRunAwaitingPendingAction :one
SELECT r.* FROM runs r WHERE r.id=$1 AND r.enterprise_id=$2
AND r.status='waiting_input' AND r.stop_reason='pending_action_confirmation'
AND EXISTS(SELECT 1 FROM conversation_events e WHERE e.run_id=r.id
    AND e.enterprise_id=r.enterprise_id AND e.event_type='pending_action_created'
    AND e.payload->>'action_ref'=sqlc.arg(action_ref)::text)
AND NOT EXISTS(SELECT 1 FROM pending_actions other WHERE other.run_id=r.id
    AND other.enterprise_id=r.enterprise_id AND other.action_ref<>sqlc.arg(action_ref)::text
    AND other.status NOT IN ('cancelled','expired','rejected','invalidated'))
FOR UPDATE;

-- name: CountRunInferenceCalls :one
SELECT count(*) FROM model_calls WHERE run_id=$1 AND enterprise_id=$2 AND call_kind='inference';

-- name: HasUnknownRunToolResult :one
SELECT EXISTS(SELECT 1 FROM tool_calls WHERE run_id=$1 AND enterprise_id=$2 AND status='result_unknown');

-- name: MarkModelDispatched :exec
UPDATE model_calls SET status='running',projection_hash=$3,tool_snapshot_hash=$4,capability_snapshot=$5,
dispatched_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='reserved';

-- name: ClaimContextRevision :one
UPDATE conversations SET context_revision=context_revision+1
WHERE id=$1 AND enterprise_id=$2 AND context_revision=$3 RETURNING context_revision;

-- name: ListConversationActionFacts :many
SELECT action.action_ref,action.status,action.resource_type,action.resource_id,
 action.result_summary,action.error_code,execution.execution_ref,execution.status AS execution_status
FROM pending_actions action JOIN runs run ON run.id=action.run_id AND run.enterprise_id=action.enterprise_id
LEFT JOIN executions execution ON execution.pending_action_id=action.id AND execution.enterprise_id=action.enterprise_id
WHERE run.conversation_id=$1 AND action.enterprise_id=$2 AND action.creator_subject_id=$3
ORDER BY action.updated_at DESC LIMIT 20;

-- name: MarkMCPManagedSecret :exec
UPDATE secrets SET owner_type='mcp_connection',owner_id=$3 WHERE id=$1 AND enterprise_id=$2;

-- name: IsRuntimeTaskLeaseCurrent :one
SELECT EXISTS(SELECT 1 FROM runtime_tasks WHERE id=$1 AND lease_owner=$2 AND fence_token=$3 AND status='running' AND lease_until>now())::boolean;

-- name: LockRuntimeTaskLease :one
SELECT id FROM runtime_tasks WHERE id=$1 AND lease_owner=$2 AND fence_token=$3 AND status='running' AND lease_until>now() FOR UPDATE;

-- name: ReconcileInterruptedModelCalls :exec
WITH interrupted AS (
 UPDATE model_calls SET status='failed',stop_reason='worker_interrupted',error_code='MODEL_RESPONSE_INTERRUPTED',completed_at=now()
 WHERE model_calls.run_id=$1 AND model_calls.enterprise_id=$2 AND model_calls.call_kind=$3 AND model_calls.status IN ('reserved','running') RETURNING model_calls.step_id
)
UPDATE run_steps SET status='failed',updated_at=now()
WHERE run_steps.id IN (SELECT step_id FROM interrupted)
OR (run_steps.status='running' AND run_steps.run_id=$1 AND run_steps.enterprise_id=$2
 AND EXISTS(SELECT 1 FROM model_calls c WHERE c.step_id=run_steps.id AND c.call_kind=$3));

-- name: GetToolResultEventSequence :one
SELECT sequence FROM conversation_events WHERE enterprise_id=$1 AND conversation_id=$2 AND event_type='tool_call_result'
AND payload->>'tool_call_id'=sqlc.arg('tool_call_id')::text ORDER BY sequence DESC LIMIT 1;
