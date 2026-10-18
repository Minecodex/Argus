-- name: GetRemoteIdentity :one
SELECT * FROM remote_execution_identities WHERE enterprise_id=$1 AND resource_type=$2 AND resource_id=$3 AND status='active';

-- name: CreateRemoteIdentity :one
INSERT INTO remote_execution_identities(id,enterprise_id,resource_type,resource_id,username,identity_sid,platform,credential_id,credential_version,connector_id,binding_source,resource_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(enterprise_id,resource_type,resource_id) DO NOTHING RETURNING *;

-- name: GetRemoteIdentityByID :one
SELECT * FROM remote_execution_identities WHERE id=$1 AND enterprise_id=$2 AND status='active';

-- name: GetRemoteIdentityForControl :one
SELECT * FROM remote_execution_identities WHERE id=$1 AND enterprise_id=$2;

-- name: RejectRemoteCommandBeforeDispatch :one
UPDATE connector_commands SET status='failed',dispatch_rejected=true,error_code=$2,completed_at=now(),updated_at=now()
WHERE id=$1 AND status='queued' AND command_type LIKE 'remote_execution_%' RETURNING *;

-- name: BindVerifiedRemoteIdentity :one
INSERT INTO remote_execution_identities(id,enterprise_id,resource_type,resource_id,username,identity_sid,platform,connector_id,binding_source,resource_version)
VALUES($1,$2,'host',$3,$4,$5,$6,$7,'verified_local_account',$8)
ON CONFLICT(enterprise_id,resource_type,resource_id) DO UPDATE SET username=excluded.username,identity_sid=excluded.identity_sid,platform=excluded.platform,connector_id=excluded.connector_id,binding_source=excluded.binding_source,resource_version=excluded.resource_version,credential_id=NULL,credential_version=0,version=remote_execution_identities.version+1,status='active'
RETURNING *;

-- name: RefreshRemoteIdentity :one
UPDATE remote_execution_identities SET credential_version=$3,resource_version=$4,
version=CASE WHEN credential_version<>$3 THEN version+1 ELSE version END
WHERE id=$1 AND enterprise_id=$2 AND status='active' RETURNING *;

-- name: CreateRemoteSession :one
INSERT INTO remote_execution_sessions(id,enterprise_id,conversation_id,owner_user_id,resource_type,resource_id,identity_id,identity_version,interpreter,target,target_hash,generation)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(enterprise_id,conversation_id,owner_user_id,resource_type,resource_id,identity_id,identity_version,interpreter,target_hash)
DO UPDATE SET last_active_at=remote_execution_sessions.last_active_at RETURNING *;

-- name: GetRemoteSession :one
SELECT * FROM remote_execution_sessions WHERE id=$1 AND enterprise_id=$2;

-- name: LockRemoteSession :one
SELECT * FROM remote_execution_sessions WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: ReserveRemoteSession :execrows
UPDATE remote_execution_sessions SET active_request_id=$3,last_active_at=now()
WHERE id=$1 AND enterprise_id=$2 AND generation=$4 AND context_revision=$5 AND status IN ('new','active')
AND (active_request_id IS NULL OR active_request_id=$3);

-- name: CompleteRemoteSessionBatch :one
UPDATE remote_execution_sessions SET context_revision=context_revision+1,completed_sequence=completed_sequence+1,
known_directory=$4,status=$5,last_active_at=now()
WHERE id=$1 AND enterprise_id=$2 AND active_request_id=$3 RETURNING *;

-- name: ReleaseRemoteRequestSessions :exec
UPDATE remote_execution_sessions SET active_request_id=NULL,last_active_at=now() WHERE enterprise_id=$1 AND active_request_id=$2;

-- name: ResetLostRemoteSession :one
UPDATE remote_execution_sessions SET generation=$3,context_revision=0,completed_sequence=0,status='new',known_directory='',runtime_account='',interpreter_version='',last_active_at=now()
WHERE id=$1 AND enterprise_id=$2 AND active_request_id IS NULL AND status IN ('lost','reclaimed','untrusted') RETURNING *;

-- name: RecordRemoteRuntimeIdentity :execrows
UPDATE remote_execution_sessions SET runtime_account=$4,interpreter_version=$5 WHERE id=$1 AND enterprise_id=$2 AND generation=$3
AND (runtime_account='' OR runtime_account=$4) AND (interpreter_version='' OR interpreter_version=$5);

-- name: ListIdleRemoteSessions :many
SELECT s.* FROM remote_execution_sessions s WHERE s.status IN ('active','new','lost','untrusted') AND s.active_request_id IS NULL
AND s.last_active_at<sqlc.arg('idle_before')
AND NOT EXISTS(SELECT 1 FROM remote_execution_files f WHERE f.session_id=s.id AND f.direction='output' AND f.status IN ('transferring','failed','partial'))
AND NOT EXISTS(SELECT 1 FROM remote_execution_steps t WHERE t.session_id=s.id AND t.status NOT IN ('pending','skipped') AND t.result->>'outputs_collected' IS DISTINCT FROM 'true')
ORDER BY s.last_active_at LIMIT 100;

-- name: MarkRemoteSessionReclaimed :execrows
UPDATE remote_execution_sessions SET status='reclaimed' WHERE id=$1 AND enterprise_id=$2 AND active_request_id IS NULL AND generation=$3 AND last_active_at=$4;

-- name: CreateRemoteRequest :one
INSERT INTO remote_execution_requests(id,enterprise_id,conversation_id,owner_user_id,run_id,tool_call_id,kind,revision,supersedes_id,recovery_of_id,action_ref,content_hash,spec,authorization_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *;

-- name: GetRemoteRequest :one
SELECT * FROM remote_execution_requests WHERE id=$1 AND enterprise_id=$2;

-- name: RecordRemoteModelContext :exec
INSERT INTO remote_execution_context_observations(model_call_id,enterprise_id,run_id,authorization_version,projection_hash,activity)
VALUES($1,$2,$3,$4,$5,$6);

-- name: RecordRemoteModelDispatchHash :exec
UPDATE remote_execution_context_observations o SET projection_hash=m.projection_hash
FROM model_calls m
WHERE o.model_call_id=m.id AND o.enterprise_id=m.enterprise_id
  AND m.id=sqlc.arg('model_call_id') AND m.enterprise_id=sqlc.arg('enterprise_id')
  AND m.status='running' AND m.dispatched_at IS NOT NULL;

-- name: GetRemoteRequestByToolCall :one
SELECT * FROM remote_execution_requests WHERE tool_call_id=$1 AND enterprise_id=$2;

-- name: GetRemoteRequestByAction :one
SELECT * FROM remote_execution_requests WHERE action_ref=$1 AND enterprise_id=$2;

-- name: LockRemoteRequest :one
SELECT * FROM remote_execution_requests WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: ListConversationRemoteRequests :many
SELECT * FROM remote_execution_requests WHERE enterprise_id=$1 AND conversation_id=$2 AND owner_user_id=$3 ORDER BY created_at DESC LIMIT 100;

-- name: ReadyRemoteRequest :execrows
UPDATE remote_execution_requests SET status='ready',updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='awaiting_confirmation' AND NOT stop_requested;

-- name: StartRemoteRequest :execrows
UPDATE remote_execution_requests SET status='running',updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='ready' AND NOT stop_requested;

-- name: FinishRemoteRequest :one
UPDATE remote_execution_requests SET status=$3,error_code=$4,updated_at=now() WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: StopRemoteRequest :one
UPDATE remote_execution_requests SET stop_requested=true,stop_status='requested',status=CASE WHEN status IN ('running','ready','result_unknown','paused_for_recovery') THEN 'stopping' WHEN status='awaiting_confirmation' THEN 'cancelled' ELSE status END,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 RETURNING *;

-- name: SupersedeRemoteRequest :execrows
UPDATE remote_execution_requests SET status='superseded',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 AND status IN ('awaiting_confirmation','failed','partial_failed','paused_for_recovery','result_unknown','cancelled','expired','superseded');

-- name: CreateRemoteStep :one
INSERT INTO remote_execution_steps(id,enterprise_id,request_id,session_id,generation,sequence) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: ListRemoteSteps :many
SELECT * FROM remote_execution_steps WHERE request_id=$1 AND enterprise_id=$2 ORDER BY sequence;

-- name: StartRemoteStep :execrows
UPDATE remote_execution_steps SET status='preparing',started_at=now(),updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='pending';

-- name: DispatchRemoteStep :execrows
UPDATE remote_execution_steps SET status='running',command_id=$3,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='preparing';

-- name: FinishRemoteStep :execrows
UPDATE remote_execution_steps SET status=$3,result=$4,completed_at=now(),updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('pending','preparing','running','collecting','result_unknown');

-- name: SaveRemoteStepReceipt :execrows
UPDATE remote_execution_steps SET status=$3,result=$4,completed_at=now(),updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('preparing','running');

-- name: SkipRemoteSteps :exec
UPDATE remote_execution_steps SET status='skipped',completed_at=now(),updated_at=now() WHERE request_id=$1 AND enterprise_id=$2 AND status='pending';

-- name: ListRemoteResourceActivity :many
SELECT r.id,r.owner_user_id,r.status,s.resource_type,s.resource_id,t.sequence,t.status AS step_status,
left(r.spec->'steps'->(t.sequence-1)->>'script',4096)::text AS script,t.updated_at
FROM remote_execution_steps t JOIN remote_execution_requests r ON r.id=t.request_id JOIN remote_execution_sessions s ON s.id=t.session_id
JOIN enterprise_users subject ON subject.id=sqlc.arg(user_id) AND subject.enterprise_id=r.enterprise_id AND subject.status='active'
JOIN departments department ON department.id=subject.department_id AND department.enterprise_id=subject.enterprise_id AND department.status='active'
WHERE r.enterprise_id=sqlc.arg(enterprise_id) AND r.conversation_id<>sqlc.arg(conversation_id)
AND (r.owner_user_id=subject.id OR sqlc.arg(can_read_others)::boolean)
AND EXISTS (
 SELECT 1 FROM data_authorization_grants dag
 WHERE dag.enterprise_id=r.enterprise_id AND dag.resource_type=s.resource_type AND dag.resource_id=s.resource_id AND dag.status='active'
 AND ((dag.subject_type='user' AND dag.subject_id=subject.id)
 OR (dag.subject_type='department' AND dag.subject_id=subject.department_id)
 OR (dag.subject_type='role' AND EXISTS (
  SELECT 1 FROM role_bindings binding JOIN roles role ON role.id=binding.role_id AND role.enterprise_id=binding.enterprise_id AND role.status='active'
  WHERE binding.enterprise_id=dag.enterprise_id AND binding.role_id=dag.subject_id AND binding.status='active'
  AND (binding.valid_from IS NULL OR binding.valid_from<=now()) AND (binding.valid_until IS NULL OR binding.valid_until>now())
  AND ((binding.subject_type='user' AND binding.subject_id=subject.id) OR (binding.subject_type='department' AND binding.subject_id=subject.department_id))
 )))
)
AND ((r.status IN ('running','stopping','result_unknown') AND t.status IN ('preparing','running','result_unknown'))
 OR (r.status='paused_for_recovery' AND t.status='paused_for_recovery' AND coalesce(t.result->>'terminated','false')<>'true'))
ORDER BY t.updated_at DESC LIMIT 16;

-- name: CreateRemoteFile :one
INSERT INTO remote_execution_files(id,enterprise_id,request_id,session_id,direction,path,byte_size,content_hash,object_key,status,source_ref)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(request_id,direction,session_id,path) DO UPDATE SET path=remote_execution_files.path RETURNING *;

-- name: ListRemoteFiles :many
SELECT * FROM remote_execution_files WHERE request_id=$1 AND enterprise_id=$2 AND direction=$3 ORDER BY path;

-- name: UpdateRemoteFileTransfer :execrows
UPDATE remote_execution_files SET transferred_bytes=$3,status=$4 WHERE id=$1 AND enterprise_id=$2 AND transferred_bytes<=$3;

-- name: MarkExecutionRemoteResultUnknown :one
UPDATE executions SET status='result_unknown',remote_execution_id=$3,updated_at=now() WHERE id=$1 AND enterprise_id=$2 RETURNING *;

-- name: SetConversationMode :one
UPDATE conversations SET mode=$4,version=version+1,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND owner_user_id=$3 AND version=$5 AND status='active' RETURNING *;

-- name: SnapshotRunMode :exec
UPDATE runs SET mode_snapshot=(SELECT mode FROM conversations WHERE conversations.id=runs.conversation_id) WHERE runs.id=$1 AND runs.enterprise_id=$2;

-- name: ListRemoteMatchingApprovalPolicies :many
SELECT * FROM approval_policies WHERE enterprise_id=sqlc.arg('enterprise_id') AND enabled
AND sqlc.arg('risk')::text=ANY(risks)
AND (cardinality(tool_ids)=0 OR tool_ids && sqlc.arg('tools')::text[])
AND (cardinality(resource_types)=0 OR resource_types && sqlc.arg('resource_types')::text[])
ORDER BY id;

-- name: StopRunRemoteRequests :exec
UPDATE remote_execution_requests SET stop_requested=true,stop_status='requested',status=CASE WHEN status IN ('running','ready') THEN 'stopping' ELSE 'cancelled' END,updated_at=now()
WHERE enterprise_id=$1 AND run_id=$2 AND status IN ('awaiting_confirmation','ready','running');

-- name: GetRemoteFile :one
SELECT * FROM remote_execution_files WHERE id=$1 AND enterprise_id=$2 AND request_id=$3;

-- name: MarkRemoteSessionLost :execrows
UPDATE remote_execution_sessions SET status='lost',last_active_at=now() WHERE id=$1 AND enterprise_id=$2 AND generation=$3;

-- name: GetLastRemoteSessionRequest :one
SELECT r.* FROM remote_execution_requests r JOIN remote_execution_steps t ON t.request_id=r.id
WHERE t.session_id=$1 AND r.enterprise_id=$2 ORDER BY r.created_at DESC LIMIT 1;

-- name: RecordRemoteStopReceipt :execrows
UPDATE remote_execution_requests SET stop_status=$3,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status IN ('running','stopping') AND stop_status<>'confirmed';

-- name: EnsureRemoteGeneration :exec
INSERT INTO remote_execution_generations(session_id,generation,enterprise_id,status,last_active_at) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(session_id,generation) DO NOTHING;

-- name: TouchRemoteGeneration :exec
UPDATE remote_execution_generations SET status=$4,last_active_at=now() WHERE session_id=$1 AND generation=$2 AND enterprise_id=$3 AND status<>'reclaimed';

-- name: ListIdleRemoteGenerations :many
SELECT g.*,s.identity_id,s.context_revision,s.active_request_id FROM remote_execution_generations g JOIN remote_execution_sessions s ON s.id=g.session_id
WHERE g.status IN ('new','active','lost','untrusted') AND g.last_active_at<sqlc.arg(idle_before)
AND (s.generation<>g.generation OR s.active_request_id IS NULL)
AND NOT EXISTS(SELECT 1 FROM remote_execution_steps t WHERE t.session_id=g.session_id AND t.generation=g.generation AND t.command_id<>'' AND (t.result->>'outputs_collected' IS DISTINCT FROM 'true' OR t.result->>'terminated' IS DISTINCT FROM 'true'))
ORDER BY g.last_active_at LIMIT 100;

-- name: ReclaimRemoteGeneration :execrows
UPDATE remote_execution_generations AS g SET status='reclaimed' WHERE g.session_id=$1 AND g.generation=$2 AND g.enterprise_id=$3 AND g.last_active_at=$4 AND g.status<>'reclaimed'
AND NOT EXISTS(SELECT 1 FROM remote_execution_sessions s WHERE s.id=$1 AND s.generation=$2 AND s.active_request_id IS NOT NULL);

-- name: GetLastRemoteGenerationRequest :one
SELECT r.* FROM remote_execution_requests r JOIN remote_execution_steps t ON t.request_id=r.id WHERE t.session_id=$1 AND t.generation=$2 AND r.enterprise_id=$3 ORDER BY r.created_at DESC LIMIT 1;

-- name: GetRemoteContextPredecessor :one
SELECT t.request_id,t.sequence FROM remote_execution_steps t
WHERE t.session_id=$1 AND t.generation=$2 AND t.enterprise_id=$3 AND t.command_id<>''
AND t.status IN ('succeeded','failed') AND t.result->>'terminated'='true'
ORDER BY t.started_at DESC,t.sequence DESC LIMIT 1;

-- name: ListLateRemoteReceipts :many
SELECT t.id,t.enterprise_id,t.request_id,t.session_id,t.generation,t.sequence,c.result,c.payload
FROM remote_execution_steps t JOIN (
 SELECT command_id,enterprise_id,result,payload,updated_at FROM connector_commands WHERE status='succeeded' AND command_type='remote_execution_batch'
 UNION ALL
 SELECT command_id,enterprise_id,result,payload,updated_at FROM remote_execution_direct_commands WHERE status='succeeded' AND command_type='remote_execution_batch'
) c ON c.command_id=t.command_id AND c.enterprise_id=t.enterprise_id
WHERE t.status='result_unknown'
ORDER BY c.updated_at LIMIT 100;

-- name: RecordLateRemoteReceipt :execrows
UPDATE remote_execution_steps SET status='collecting',result=$3,updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='result_unknown';

-- name: PauseReconciledRemoteRequest :exec
UPDATE remote_execution_requests SET status='paused_for_recovery',error_code='REMOTE_CONTEXT_LOST',updated_at=now() WHERE id=$1 AND enterprise_id=$2 AND status='result_unknown' AND NOT stop_requested;

-- name: GetRemoteFeedbackParent :one
SELECT r.id FROM conversation_events e JOIN remote_execution_requests r ON r.id::text=e.payload->>'revision_of'
WHERE e.enterprise_id=sqlc.arg(enterprise_id) AND e.conversation_id=sqlc.arg(conversation_id) AND e.run_id=sqlc.arg(run_id)
AND e.event_type='user_message' AND e.actor_type='user' AND r.owner_user_id=sqlc.arg(user_id) AND r.kind='plan' AND r.status='superseded'
AND NOT EXISTS(SELECT 1 FROM remote_execution_requests child WHERE child.supersedes_id=r.id AND child.tool_call_id<>sqlc.arg(tool_call_id))
ORDER BY e.sequence DESC LIMIT 1;
