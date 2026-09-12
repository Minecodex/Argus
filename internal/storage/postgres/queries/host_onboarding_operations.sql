-- name: CreateHostOnboardingOperation :one
INSERT INTO host_onboarding_operations (id,enterprise_id,host_id,connector_id,pending_action_id,retry_of,release_version_id,connection_test_id,install_method,ssh_path,target_platform,control_path,bastion_scope_id,plan,plan_hash,expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING *;

-- name: GetHostOnboardingOperation :one
SELECT * FROM host_onboarding_operations WHERE id=$1 AND enterprise_id=$2;

-- name: SetHostOnboardingRetryOf :exec
UPDATE host_onboarding_operations SET retry_of=$3 WHERE id=$1 AND enterprise_id=$2;

-- name: ListRunningBastionHostOnboardingOperations :many
SELECT * FROM host_onboarding_operations WHERE status='running' AND ssh_path='bastion_connector' ORDER BY updated_at,id;

-- name: GetLatestHostOnboardingOperation :one
SELECT * FROM host_onboarding_operations WHERE host_id=$1 AND enterprise_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: GetLatestSuccessfulHostOnboardingOperation :one
SELECT * FROM host_onboarding_operations
WHERE host_id=$1 AND enterprise_id=$2 AND status='succeeded'
ORDER BY completed_at DESC NULLS LAST,created_at DESC,id DESC LIMIT 1;

-- name: GetLatestHostOnboardingOperationByConnector :one
SELECT * FROM host_onboarding_operations
WHERE host_id=$1 AND connector_id=$2 AND enterprise_id=$3
ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: GetActiveHostOnboardingOperationByConnector :one
SELECT * FROM host_onboarding_operations
WHERE connector_id=$1 AND enterprise_id=$2 AND status IN ('queued','running')
ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: CancelHostOnboardingOperationsByHost :execrows
UPDATE host_onboarding_operations SET status='cancelled',error_code='HOST_ONBOARDING_CANCELLED_BY_DELETE',
 completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE host_id=$1 AND enterprise_id=$2 AND status IN ('queued','running','result_unknown');

-- name: DeleteHostOnboardingOperationSecretsByHost :execrows
DELETE FROM host_onboarding_operation_secrets secret
USING host_onboarding_operations operation
WHERE secret.operation_id=operation.id AND secret.enterprise_id=operation.enterprise_id
  AND operation.host_id=$1 AND operation.enterprise_id=$2;

-- name: RevokeHostOnboardingCredentialLeasesByHost :execrows
UPDATE credential_leases lease SET status='revoked'
WHERE lease.enterprise_id=$2 AND lease.status='active' AND lease.operation_ref IN (
  SELECT operation.id::text FROM host_onboarding_operations operation WHERE operation.host_id=$1 AND operation.enterprise_id=$2
  UNION ALL
  SELECT 'host_onboarding:'||operation.id::text FROM host_onboarding_operations operation WHERE operation.host_id=$1 AND operation.enterprise_id=$2
);

-- name: ExpireHostOnboardingConnectorCommandsByHost :execrows
UPDATE connector_commands command SET status='expired',error_code='HOST_ONBOARDING_CANCELLED_BY_DELETE',completed_at=now(),updated_at=now()
WHERE command.enterprise_id=$2 AND command.status IN ('queued','dispatched','acknowledged','running','delivery_unknown','result_unknown')
  AND command.operation_ref IN (SELECT operation.id::text FROM host_onboarding_operations operation WHERE operation.host_id=$1 AND operation.enterprise_id=$2);

-- name: RevokeHostControlTunnelLeasesByHost :execrows
UPDATE credential_leases lease SET status='revoked'
WHERE lease.enterprise_id=$2 AND lease.status='active' AND lease.operation_ref IN (
  SELECT 'connector_control_tunnel:'||tunnel.id::text FROM connector_control_tunnels tunnel WHERE tunnel.host_id=$1 AND tunnel.enterprise_id=$2
);

-- name: MarkHostControlTunnelsRemovedByHost :execrows
UPDATE connector_control_tunnels SET status='removed',last_drop_reason='host_onboarding_cancelled_by_delete',epoch=epoch+1,
 lease_owner='',lease_expires_at=NULL,next_claim_at=NULL,updated_at=now()
WHERE host_id=$1 AND enterprise_id=$2 AND status<>'removed';

-- name: ClaimHostOnboardingOperations :many
WITH claimed AS (
 SELECT id FROM host_onboarding_operations WHERE status='queued' AND ssh_path='direct_executor' AND expires_at>now()
 ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE host_onboarding_operations operation SET status='running',stage='probing',attempts=attempts+1,lease_owner=$2,lease_expires_at=now()+interval '90 seconds',updated_at=now()
FROM claimed WHERE operation.id=claimed.id RETURNING operation.*;

-- name: ClaimBastionHostOnboardingOperations :many
WITH claimed AS (
 SELECT id FROM host_onboarding_operations WHERE status='queued' AND ssh_path='bastion_connector' AND expires_at>now()
 ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE host_onboarding_operations operation SET status='running',stage='probing',attempts=attempts+1,lease_owner=$2,lease_expires_at=now()+interval '90 seconds',updated_at=now()
FROM claimed WHERE operation.id=claimed.id RETURNING operation.*;

-- name: RenewHostOnboardingOperationLease :execrows
UPDATE host_onboarding_operations SET lease_expires_at=now()+interval '90 seconds',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='running' AND lease_owner=$3;

-- name: AdvanceHostOnboardingOperation :one
UPDATE host_onboarding_operations SET status='running',stage=$3,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('queued','running') RETURNING *;

-- name: CompleteHostOnboardingOperation :one
UPDATE host_onboarding_operations SET status='succeeded',stage='completed',connector_online_at=now(),completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='running' RETURNING *;

-- name: FailHostOnboardingOperation :one
UPDATE host_onboarding_operations SET status='failed',error_code=$3,completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status IN ('queued','running') RETURNING *;

-- name: RetryHostOnboardingOperation :one
UPDATE host_onboarding_operations SET status='queued',lease_owner='',lease_expires_at=NULL,error_code=$3,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status='running' AND attempts<3 AND expires_at>now() RETURNING *;

-- name: RecoverHostOnboardingOperations :execrows
UPDATE host_onboarding_operations SET status='queued',lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE status='running' AND lease_expires_at<=now() AND attempts<3 AND expires_at>now();

-- name: ExpireHostOnboardingOperations :execrows
UPDATE host_onboarding_operations SET status='expired',error_code='HOST_ONBOARDING_EXPIRED',completed_at=now(),lease_owner='',lease_expires_at=NULL,updated_at=now()
WHERE status IN ('queued','running') AND expires_at<=now();

-- name: CreateHostOnboardingOperationEvent :one
INSERT INTO host_onboarding_operation_events (id,operation_id,enterprise_id,sequence,stage,status,error_code)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: ListHostOnboardingOperationEvents :many
SELECT * FROM host_onboarding_operation_events WHERE operation_id=$1 AND enterprise_id=$2 ORDER BY sequence;

-- name: CreateHostOnboardingOperationSecret :one
INSERT INTO host_onboarding_operation_secrets (operation_id,enterprise_id,key_version,nonce,ciphertext,expires_at)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: GetHostOnboardingOperationSecret :one
SELECT * FROM host_onboarding_operation_secrets WHERE operation_id=$1 AND enterprise_id=$2 AND consumed_at IS NULL AND expires_at>now();

-- name: ConsumeHostOnboardingOperationSecret :execrows
UPDATE host_onboarding_operation_secrets SET consumed_at=now() WHERE operation_id=$1 AND enterprise_id=$2 AND consumed_at IS NULL;
