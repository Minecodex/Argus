-- name: RegisterRemoteDirectWorker :exec
INSERT INTO remote_execution_direct_workers(id) VALUES($1) ON CONFLICT(id) DO UPDATE SET last_seen_at=now();

-- name: StopRemoteDirectWorker :exec
UPDATE remote_execution_direct_workers SET last_seen_at='1970-01-01' WHERE id=$1;

-- name: CreateRemoteDirectCommand :one
INSERT INTO remote_execution_direct_commands(id,command_id,enterprise_id,identity_id,session_id,generation,command_type,payload,payload_hash,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetRemoteDirectCommand :one
SELECT * FROM remote_execution_direct_commands WHERE id=$1 AND enterprise_id=$2;

-- name: ClaimRemoteDirectCommands :many
WITH candidates AS (
 SELECT c.id FROM remote_execution_direct_commands c JOIN remote_execution_generations g ON g.session_id=c.session_id AND g.generation=c.generation
 WHERE c.status='queued' AND c.expires_at>now() AND (g.direct_owner='' OR g.direct_owner=sqlc.arg(owner_instance)
 OR c.command_type='remote_execution_control' AND c.payload->>'operation' IN ('recover_files','reclaim')
 AND NOT EXISTS(SELECT 1 FROM remote_execution_direct_workers w WHERE w.id=g.direct_owner AND w.last_seen_at>now()-interval '15 seconds'))
 AND (NOT sqlc.arg(stops_only)::boolean OR c.command_type='remote_execution_control' AND c.payload->>'operation'='stop')
 ORDER BY CASE WHEN c.command_type='remote_execution_control' AND c.payload->>'operation'='stop' THEN 0 ELSE 1 END,c.created_at
 LIMIT sqlc.arg(row_limit) FOR UPDATE OF c SKIP LOCKED
)
UPDATE remote_execution_direct_commands SET status='running',owner_instance=sqlc.arg(owner_instance),updated_at=now()
WHERE id IN (SELECT id FROM candidates) RETURNING *;

-- name: BindRemoteDirectOwner :execrows
UPDATE remote_execution_generations SET direct_owner=$3 WHERE session_id=$1 AND generation=$2 AND (direct_owner='' OR direct_owner=$3);

-- name: BindRemoteDirectRecoveryOwner :execrows
UPDATE remote_execution_generations g SET direct_owner=$3 WHERE session_id=$1 AND generation=$2
AND (direct_owner='' OR direct_owner=$3 OR NOT EXISTS(SELECT 1 FROM remote_execution_direct_workers w WHERE w.id=g.direct_owner AND w.last_seen_at>now()-interval '15 seconds'));

-- name: CompleteRemoteDirectCommand :execrows
UPDATE remote_execution_direct_commands SET status=$4,result=$5,error_code=$6,dispatch_rejected=$7,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND owner_instance=$3 AND status IN ('running','result_unknown');

-- name: RequeueUnownedRemoteDirectCommand :exec
UPDATE remote_execution_direct_commands SET status='queued',owner_instance='',updated_at=now() WHERE id=$1 AND owner_instance=$2 AND status='running';

-- name: RecoverLostRemoteDirectCommands :exec
UPDATE remote_execution_direct_commands c SET status=CASE WHEN c.status='queued' THEN 'failed' ELSE 'result_unknown' END,
dispatch_rejected=(c.status='queued'),error_code='REMOTE_CONTEXT_LOST',updated_at=now()
FROM remote_execution_generations g
WHERE c.session_id=g.session_id AND c.generation=g.generation AND c.status IN ('queued','running')
AND (c.expires_at<=now() OR g.direct_owner<>'' AND NOT EXISTS(SELECT 1 FROM remote_execution_direct_workers w WHERE w.id=g.direct_owner AND w.last_seen_at>now()-interval '15 seconds')
AND NOT (c.status='queued' AND c.command_type='remote_execution_control' AND c.payload->>'operation' IN ('recover_files','reclaim')));

-- name: MarkLostRemoteDirectSessions :exec
UPDATE remote_execution_sessions s SET status='lost' FROM remote_execution_generations g
WHERE s.id=g.session_id AND s.generation=g.generation AND s.status IN ('new','active') AND g.direct_owner<>''
AND NOT EXISTS(SELECT 1 FROM remote_execution_direct_workers w WHERE w.id=g.direct_owner AND w.last_seen_at>now()-interval '15 seconds');
