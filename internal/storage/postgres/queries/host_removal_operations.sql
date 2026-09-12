-- name: StartManagedHostRemoval :one
UPDATE hosts SET status='draining', local_cleanup='pending', removal_generation=removal_generation+1,
 resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND resource_version=$3 AND connector_id=$4
  AND role='managed_host' AND status IN ('active','disabled','removal_failed','cleanup_unknown')
RETURNING *;

-- name: StartBastionScopeRemoval :one
UPDATE bastion_scopes SET status='draining', local_cleanup='pending', removal_generation=removal_generation+1,
 resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND resource_version=$3 AND active_connector_id=$4
  AND status IN ('active','suspected_offline','offline','removal_failed','cleanup_unknown')
RETURNING *;

-- name: StartBastionRootHostRemoval :one
UPDATE hosts SET status='draining',local_cleanup='pending',removal_generation=$4,
 resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND role='bastion'
  AND status IN ('active','disabled','removal_failed','cleanup_unknown') RETURNING *;

-- name: MarkRemovalHostUninstalling :execrows
UPDATE hosts SET status='uninstalling', resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND removal_generation=$4
  AND status IN ('draining','uninstalling');

-- name: ResumeManagedHostRemoval :execrows
UPDATE hosts SET status='draining',local_cleanup='pending',resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND removal_generation=$4
  AND status IN ('removal_failed','cleanup_unknown');

-- name: ResumeBastionScopeRemoval :execrows
UPDATE bastion_scopes SET status='draining',local_cleanup='pending',resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND active_connector_id=$3 AND removal_generation=$4
  AND status IN ('removal_failed','cleanup_unknown');

-- name: ResumeBastionRootHostRemoval :execrows
UPDATE hosts SET status='draining',local_cleanup='pending',resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND removal_generation=$4
  AND role='bastion' AND status IN ('removal_failed','cleanup_unknown');

-- name: MarkRemovalBastionUninstalling :execrows
UPDATE bastion_scopes SET status='uninstalling', resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND active_connector_id=$3 AND removal_generation=$4
  AND status IN ('draining','uninstalling');

-- name: MarkRemovalHostTerminal :execrows
UPDATE hosts SET status=$5, local_cleanup=$6, connection_status='offline',
 resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND removal_generation=$4
  AND status IN ('draining','uninstalling','removal_failed','cleanup_unknown');

-- name: MarkRemovalBastionTerminal :execrows
UPDATE bastion_scopes SET status=$5, local_cleanup=$6, relay_status='offline',
 active_connector_id=CASE WHEN $5='uninstalled' THEN NULL ELSE active_connector_id END,
 resource_version=resource_version+1, updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND active_connector_id=$3 AND removal_generation=$4
  AND status IN ('draining','uninstalling','removal_failed','cleanup_unknown');

-- name: MarkRemovalBastionHostTerminal :execrows
UPDATE hosts SET status=$5,local_cleanup=$6,connection_status='offline',resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND connector_id=$3 AND removal_generation=$4
  AND role='bastion' AND status IN ('draining','uninstalling','removal_failed','cleanup_unknown');

-- name: CreateHostRemovalOperation :one
INSERT INTO host_removal_operations (
 id,enterprise_id,pending_action_id,target_type,host_id,bastion_scope_id,connector_id,removal_mode,
 delivery_method,ssh_path,target_platform,control_path,connection_test_id,credential_id,credential_version,
 pinned_host_key,resource_version,connector_version,connection_epoch,removal_generation,trust_bundle_epoch,
 plan,plan_hash,status,stage,expires_at
) VALUES (
 $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26
) RETURNING *;

-- name: SupersedeHostRemovalOperations :execrows
UPDATE host_removal_operations SET status='failed',error_code='HOST_REMOVAL_SUPERSEDED',completed_at=now(),
 lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE enterprise_id=$1 AND host_id=$2 AND status IN ('queued','running','awaiting_manual_execution','failed','cleanup_unknown');

-- name: GetHostRemovalOperation :one
SELECT * FROM host_removal_operations WHERE id=$1 AND enterprise_id=$2;

-- name: SupersedeHostRemovalCommands :execrows
UPDATE connector_commands SET status='failed',error_code='CONNECTOR_COMMAND_SUPERSEDED',completed_at=now(),updated_at=now()
WHERE enterprise_id=$1 AND operation_ref=$2 AND command_type='host_connector_removal'
  AND status IN ('delivery_unknown','result_unknown');

-- name: GetLatestHostRemovalOperation :one
SELECT * FROM host_removal_operations WHERE host_id=$1 AND enterprise_id=$2
ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: GetHostRemovalOperationForUpdate :one
SELECT * FROM host_removal_operations WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: ClaimDirectHostRemovalOperations :many
WITH claimed AS (
 SELECT id FROM host_removal_operations
 WHERE status='queued' AND delivery_method='ssh' AND ssh_path='direct_executor' AND expires_at>now()
 ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE host_removal_operations operation SET status='running',stage='terminating_sessions',attempts=attempts+1,
 lease_owner=$2,lease_expires_at=now()+interval '90 seconds',updated_at=now()
FROM claimed WHERE operation.id=claimed.id RETURNING operation.*;

-- name: ClaimBastionHostRemovalOperations :many
WITH claimed AS (
 SELECT operation.id FROM host_removal_operations operation
 JOIN bastion_scopes scope ON scope.id=operation.bastion_scope_id AND scope.enterprise_id=operation.enterprise_id
 WHERE operation.status='queued' AND operation.delivery_method='ssh' AND operation.ssh_path='bastion_connector'
   AND operation.expires_at>now() AND scope.status='active' AND scope.active_connector_id IS NOT NULL
 ORDER BY operation.created_at,operation.id LIMIT $1 FOR UPDATE OF operation SKIP LOCKED
)
UPDATE host_removal_operations operation SET status='running',stage='terminating_sessions',attempts=attempts+1,
 lease_owner=$2,lease_expires_at=now()+interval '90 seconds',updated_at=now()
FROM claimed WHERE operation.id=claimed.id RETURNING operation.*;

-- name: ListRunningBastionHostRemovalOperations :many
SELECT * FROM host_removal_operations
WHERE status='running' AND delivery_method='ssh' AND ssh_path='bastion_connector'
ORDER BY updated_at,id;

-- name: ClaimServerOnlyHostRemovalOperations :many
WITH claimed AS (
 SELECT id FROM host_removal_operations
 WHERE status='queued' AND delivery_method='server_only' AND expires_at>now()
 ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE host_removal_operations operation SET status='running',stage='terminating_sessions',attempts=attempts+1,
 lease_owner=$2,lease_expires_at=now()+interval '90 seconds',updated_at=now()
FROM claimed WHERE operation.id=claimed.id RETURNING operation.*;

-- name: RenewHostRemovalOperationLease :execrows
UPDATE host_removal_operations SET lease_expires_at=now()+interval '90 seconds',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='running' AND lease_owner=$3;

-- name: AdvanceHostRemovalOperation :one
UPDATE host_removal_operations SET status=$3,stage=$4,error_code=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('queued','running','awaiting_manual_execution','failed','cleanup_unknown')
RETURNING *;

-- name: CompleteHostRemovalOperation :one
UPDATE host_removal_operations SET status='succeeded',stage='completed',local_cleanup=$3,error_code=$4,
 completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('running','awaiting_manual_execution','cleanup_unknown') RETURNING *;

-- name: FailHostRemovalOperation :one
UPDATE host_removal_operations SET status=$3,error_code=$4,local_cleanup=$5,completed_at=now(),
 lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('queued','running','awaiting_manual_execution') RETURNING *;

-- name: RetryHostRemovalOperation :one
UPDATE host_removal_operations SET status=$3,stage=$4,error_code=NULL,completed_at=NULL,
 lease_owner='',lease_expires_at=NULL,expires_at=$5,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('failed','cleanup_unknown','awaiting_manual_execution') AND attempts<10 RETURNING *;

-- name: RecoverHostRemovalOperations :execrows
UPDATE host_removal_operations SET status='queued',lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE status='running' AND delivery_method<>'manual' AND lease_expires_at<=now() AND attempts<10 AND expires_at>now();

-- name: ExpireHostRemovalOperations :many
UPDATE host_removal_operations SET status=CASE WHEN status='running' THEN 'cleanup_unknown' ELSE 'failed' END,
 error_code='HOST_REMOVAL_EXPIRED',local_cleanup='unknown',
 completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE status IN ('queued','running','awaiting_manual_execution') AND expires_at<=now() RETURNING *;

-- name: UpsertHostRemovalStep :one
INSERT INTO host_removal_operation_steps (operation_id,enterprise_id,stage,status,attempt,postcondition,result_hash,error_code,started_at,completed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $4='running' THEN now() ELSE NULL END,
 CASE WHEN $4 IN ('succeeded','failed','unknown') THEN now() ELSE NULL END)
ON CONFLICT (operation_id,stage) DO UPDATE SET status=EXCLUDED.status,attempt=EXCLUDED.attempt,
 postcondition=EXCLUDED.postcondition,result_hash=EXCLUDED.result_hash,error_code=EXCLUDED.error_code,
 started_at=COALESCE(host_removal_operation_steps.started_at,EXCLUDED.started_at),completed_at=EXCLUDED.completed_at,updated_at=now()
RETURNING *;

-- name: ListHostRemovalSteps :many
SELECT * FROM host_removal_operation_steps WHERE operation_id=$1 AND enterprise_id=$2 ORDER BY updated_at,stage;

-- name: CreateHostRemovalEvent :one
INSERT INTO host_removal_operation_events (id,operation_id,enterprise_id,sequence,stage,status,error_code)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: ListHostRemovalEvents :many
SELECT * FROM host_removal_operation_events WHERE operation_id=$1 AND enterprise_id=$2 ORDER BY sequence;

-- name: NextHostRemovalEventSequence :one
SELECT COALESCE(max(sequence),0)::bigint+1 FROM host_removal_operation_events WHERE operation_id=$1;

-- name: CreateHostRemovalToken :one
INSERT INTO host_removal_tokens (id,operation_id,enterprise_id,purpose,token_hash,key_version,nonce,ciphertext,expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: GetActiveHostRemovalToken :one
SELECT * FROM host_removal_tokens
WHERE token_hash=$1 AND ((status='active' AND expires_at>now()) OR status='consumed') FOR UPDATE;

-- name: ConsumeHostRemovalToken :execrows
UPDATE host_removal_tokens SET status='consumed',consumed_at=now()
WHERE id=$1 AND operation_id=$2 AND status='active' AND expires_at>now();

-- name: RevokeHostRemovalTokens :execrows
UPDATE host_removal_tokens SET status='revoked'
WHERE operation_id=$1 AND enterprise_id=$2 AND status='active';

-- name: ListHostRemovalDependencies :many
SELECT 'remote_session'::text AS dependency_type, session.id AS dependency_id,
       session.protocol::text AS name, session.status::text AS reason
FROM remote_access_sessions session
WHERE session.enterprise_id=$1 AND session.host_id=$2 AND session.status IN ('authorized','connecting','active','terminating')
UNION ALL
SELECT 'collector'::text, collector.id, collector.role::text, collector.status::text
FROM collector_instances collector
WHERE collector.enterprise_id=$1 AND collector.resource_type='host' AND collector.resource_id=$2
  AND collector.status<>'uninstalled'
UNION ALL
SELECT 'control_tunnel'::text, tunnel.id, tunnel.target_address::text, tunnel.status::text
FROM connector_control_tunnels tunnel
WHERE tunnel.enterprise_id=$1 AND tunnel.host_id=$2 AND tunnel.status<>'removed'
UNION ALL
SELECT 'telemetry_tunnel'::text, tunnel.id, tunnel.target_address::text, tunnel.status::text
FROM telemetry_tunnels tunnel
WHERE tunnel.enterprise_id=$1 AND tunnel.host_id=$2 AND tunnel.status<>'removed'
UNION ALL
SELECT 'collector_operation'::text, operation.id, operation.operation::text, operation.status::text
FROM telemetry_collector_operations operation
JOIN collector_instances collector ON collector.id=operation.collector_id AND collector.enterprise_id=operation.enterprise_id
WHERE operation.enterprise_id=$1 AND collector.resource_type='host' AND collector.resource_id=$2
  AND operation.status IN ('queued','running','result_unknown');

-- name: ListBastionRemovalDependencies :many
SELECT 'member_host'::text AS dependency_type, host.id AS dependency_id, host.name::text AS name,
       host.control_path::text AS reason
FROM hosts host WHERE host.enterprise_id=$1 AND host.bastion_scope_id=$2 AND host.role='managed_host' AND host.status<>'deleted'
UNION ALL
SELECT 'onboarding_operation'::text, operation.id, operation.ssh_path::text, operation.status::text
FROM host_onboarding_operations operation WHERE operation.enterprise_id=$1 AND operation.bastion_scope_id=$2
  AND operation.status IN ('queued','running')
UNION ALL
SELECT 'removal_operation'::text, operation.id, operation.ssh_path::text, operation.status::text
FROM host_removal_operations operation JOIN hosts host ON host.id=operation.host_id AND host.enterprise_id=operation.enterprise_id
WHERE operation.enterprise_id=$1 AND host.bastion_scope_id=$2
  AND host.role='managed_host'
  AND operation.status IN ('queued','running','awaiting_manual_execution','cleanup_unknown')
UNION ALL
SELECT 'collector'::text, collector.id, collector.role::text, collector.status::text
FROM collector_instances collector
WHERE collector.enterprise_id=$1 AND collector.resource_type='host' AND collector.resource_id=$3 AND collector.status<>'uninstalled'
UNION ALL
SELECT 'telemetry_route'::text, route.id, collector.role::text, route.status::text
FROM telemetry_routes route JOIN collector_instances collector ON collector.id=route.collector_id AND collector.enterprise_id=route.enterprise_id
WHERE route.enterprise_id=$1 AND (collector.resource_id=$3 OR route.gateway_collector_id IN (
  SELECT id FROM collector_instances WHERE enterprise_id=$1 AND resource_type='host' AND resource_id=$3
)) AND route.status<>'invalidated'
UNION ALL
SELECT 'remote_session'::text, session.id, session.protocol::text, session.status::text
FROM remote_access_sessions session JOIN hosts host ON host.id=session.host_id AND host.enterprise_id=session.enterprise_id
WHERE session.enterprise_id=$1 AND host.bastion_scope_id=$2 AND session.status IN ('authorized','connecting','active','terminating')
UNION ALL
SELECT 'connector_command'::text, command.id, command.command_type::text, command.status::text
FROM connector_commands command JOIN bastion_scopes scope ON scope.active_connector_id=command.connector_id AND scope.enterprise_id=command.enterprise_id
WHERE command.enterprise_id=$1 AND scope.id=$2 AND command.status IN ('queued','dispatched','acknowledged','running','delivery_unknown','result_unknown')
UNION ALL
SELECT 'credential_lease'::text, lease.id, lease.protocol::text, lease.status::text
FROM credential_leases lease JOIN bastion_scopes scope ON scope.enterprise_id=lease.enterprise_id AND scope.active_connector_id::text=lease.recipient_id
WHERE lease.enterprise_id=$1 AND scope.id=$2 AND lease.recipient_type='connector' AND lease.status='active' AND lease.expires_at>now()
UNION ALL
SELECT 'control_tunnel'::text, tunnel.id, tunnel.target_address::text, tunnel.status::text
FROM connector_control_tunnels tunnel WHERE tunnel.enterprise_id=$1 AND tunnel.bastion_scope_id=$2
  AND tunnel.host_id<>$3 AND tunnel.status<>'removed'
UNION ALL
SELECT 'telemetry_tunnel'::text, tunnel.id, tunnel.target_address::text, tunnel.status::text
FROM telemetry_tunnels tunnel
LEFT JOIN hosts host ON host.id=tunnel.host_id AND host.enterprise_id=tunnel.enterprise_id
WHERE tunnel.enterprise_id=$1 AND tunnel.status<>'removed' AND (
  tunnel.host_id=$3 OR host.bastion_scope_id=$2 OR tunnel.connector_id=(
    SELECT active_connector_id FROM bastion_scopes WHERE enterprise_id=$1 AND id=$2
  )
)
UNION ALL
SELECT 'collector_operation'::text, operation.id, operation.operation::text, operation.status::text
FROM telemetry_collector_operations operation
JOIN collector_instances collector ON collector.id=operation.collector_id AND collector.enterprise_id=operation.enterprise_id
LEFT JOIN hosts host ON host.id=collector.resource_id AND host.enterprise_id=collector.enterprise_id AND collector.resource_type='host'
WHERE operation.enterprise_id=$1 AND operation.status IN ('queued','running','result_unknown')
  AND collector.resource_type='host' AND (collector.resource_id=$3 OR host.bastion_scope_id=$2);

-- name: TerminateRemoteAccessSessionsByHostRemoval :execrows
UPDATE remote_access_sessions SET status='terminating',session_fence=session_fence+1,
 termination_reason='host_removal',updated_at=now()
WHERE enterprise_id=$1 AND host_id=$2 AND status IN ('authorized','connecting','active');

-- name: InvalidateHostTelemetryForRemoval :execrows
WITH affected AS (
 UPDATE collector_instances SET status='uninstalled',version=version+1,updated_at=now()
 WHERE enterprise_id=$1 AND resource_type='host' AND resource_id=$2 AND status<>'uninstalled'
 RETURNING id
)
UPDATE telemetry_routes route SET status='invalidated',version=route.version+1,updated_at=now()
WHERE route.enterprise_id=$1 AND (route.collector_id IN (SELECT affected.id FROM affected) OR route.gateway_collector_id IN (SELECT affected.id FROM affected));

-- name: FinalizeHostConnectorRemoval :execrows
UPDATE connectors SET status='uninstalled',connection_epoch=connection_epoch+1,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$3 AND status<>'revoked';

-- name: RevokeRemovalConnectorCommands :execrows
UPDATE connector_commands SET status='expired',error_code='HOST_REMOVAL_DRAINING',completed_at=now(),updated_at=now()
WHERE enterprise_id=$1 AND connector_id=$2 AND status IN ('queued','dispatched','acknowledged','running');


-- name: RevokeHostRemovalCredentialLeases :execrows
UPDATE credential_leases SET status='revoked'
WHERE enterprise_id=$1 AND target_resource_type='host' AND target_resource_id=$2 AND status='active';

-- name: DeleteUninstalledManagedHost :one
UPDATE hosts SET status='deleted',deleted_at=now(),resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND resource_version=$3 AND role='managed_host' AND status='uninstalled'
RETURNING *;

-- name: DeleteUninstalledBastionScope :one
UPDATE bastion_scopes scope SET status='deleted',deleted_at=now(),resource_version=scope.resource_version+1,updated_at=now()
WHERE scope.id=$1 AND scope.enterprise_id=$2 AND scope.resource_version=$3 AND scope.status='uninstalled'
  AND NOT EXISTS (SELECT 1 FROM hosts host WHERE host.enterprise_id=$2 AND host.bastion_scope_id=$1 AND host.role='managed_host' AND host.status<>'deleted')
RETURNING *;

-- name: DeleteUninstalledBastionHost :execrows
UPDATE hosts SET status='deleted',deleted_at=now(),resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND role='bastion' AND status='uninstalled';

-- name: ForceForgetManagedHost :one
UPDATE hosts SET status='deleted',local_cleanup='unknown',connection_status='offline',deleted_at=now(),
 removal_generation=removal_generation+1,resource_version=resource_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND resource_version=$3 AND role='managed_host'
  AND (connection_status='offline' OR status IN ('removal_failed','cleanup_unknown')) RETURNING *;

-- name: ForceForgetBastionScope :one
UPDATE bastion_scopes scope SET status='deleted',local_cleanup='unknown',relay_status='offline',deleted_at=now(),
 removal_generation=scope.removal_generation+1,resource_version=scope.resource_version+1,updated_at=now()
WHERE scope.id=$1 AND scope.enterprise_id=$2 AND scope.resource_version=$3
  AND scope.status IN ('offline','removal_failed','cleanup_unknown')
  AND NOT EXISTS (SELECT 1 FROM hosts host WHERE host.enterprise_id=$2 AND host.bastion_scope_id=$1 AND host.role='managed_host' AND host.status<>'deleted')
RETURNING *;

-- name: UpsertHostManagedChangeJournal :one
INSERT INTO host_managed_change_journals (id,enterprise_id,host_id,connector_id,change_type,before_state,applied_state,state_hash)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (enterprise_id,host_id,change_type) WHERE status='active'
DO UPDATE SET connector_id=EXCLUDED.connector_id,before_state=EXCLUDED.before_state,applied_state=EXCLUDED.applied_state,
 state_hash=EXCLUDED.state_hash,updated_at=now() RETURNING *;

-- name: GetActiveHostManagedChangeJournal :one
SELECT * FROM host_managed_change_journals WHERE enterprise_id=$1 AND host_id=$2 AND change_type=$3 AND status='active';

-- name: CompleteHostManagedChangeJournal :execrows
UPDATE host_managed_change_journals SET status=$4,restored_at=now(),updated_at=now()
WHERE enterprise_id=$1 AND host_id=$2 AND change_type=$3 AND status='active';
