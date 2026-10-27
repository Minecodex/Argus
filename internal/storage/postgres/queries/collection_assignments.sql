-- name: PrepareMonitoringAssignment :exec
INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id,approved_action_id)
VALUES($1,$2,$3,$4,$5);

-- name: AbortPendingMonitoringAssignments :exec
UPDATE monitoring_collection_assignments SET valid_to=now()
WHERE enterprise_id=$1 AND collection_id=$2 AND activated_at IS NULL AND valid_to IS NULL;

-- name: GetMonitoringAssignment :one
SELECT * FROM monitoring_collection_assignments WHERE enterprise_id=$1 AND collection_id=$2 AND epoch=$3;

-- name: GetMonitoringAssignmentApproval :one
SELECT * FROM pending_actions WHERE enterprise_id=$1 AND id=$2;

-- name: EndOtherActiveMonitoringAssignments :exec
UPDATE monitoring_collection_assignments SET valid_to=now()
WHERE enterprise_id=$1 AND collection_id=$2 AND epoch<>$3 AND activated_at IS NOT NULL AND valid_to IS NULL;

-- name: ActivateMonitoringAssignment :execrows
UPDATE monitoring_collection_assignments SET activated_at=COALESCE(activated_at,now())
WHERE enterprise_id=$1 AND collection_id=$2 AND epoch=$3 AND collector_id=$4 AND valid_to IS NULL;

-- name: CreateMonitoringAssignmentRetirement :one
INSERT INTO monitoring_assignment_retirements(id,enterprise_id,collection_id,assignment_epoch,previous_collector_id,approved_action_id)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(collection_id,assignment_epoch,previous_collector_id) DO UPDATE SET updated_at=monitoring_assignment_retirements.updated_at RETURNING *;

-- name: LockMonitoringAssignmentRetirement :one
SELECT * FROM monitoring_assignment_retirements WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: GetMonitoringAssignmentRetirement :one
SELECT * FROM monitoring_assignment_retirements WHERE enterprise_id=$1 AND id=$2;

-- name: HasCollectorMonitoringDependencies :one
SELECT EXISTS(SELECT 1 FROM monitoring_collections c
 LEFT JOIN monitoring_collection_revisions r ON r.enterprise_id=c.enterprise_id AND r.collection_id=c.id AND r.revision=c.effective_revision
 WHERE c.enterprise_id=$1 AND ((c.collector_id=$2 AND c.status='active' AND c.desired_enabled)
 OR (c.effective_collector_id=$2 AND r.desired_enabled)))
 OR EXISTS(SELECT 1 FROM monitoring_ingest_endpoints WHERE enterprise_id=$1 AND collector_id=$2 AND enabled)
 OR EXISTS(SELECT 1 FROM monitoring_assignment_retirements WHERE enterprise_id=$1 AND previous_collector_id=$2 AND status<>'completed') AS in_use;

-- name: UpdateMonitoringAssignmentRetirement :exec
UPDATE monitoring_assignment_retirements SET status=$3,error_code=$4,collector_operation_id=$5,updated_at=now()
WHERE enterprise_id=$1 AND id=$2;

-- name: MonitoringEffectiveCredentialInUse :one
SELECT EXISTS(SELECT 1 FROM monitoring_collections c JOIN monitoring_collection_revisions r
 ON r.enterprise_id=c.enterprise_id AND r.collection_id=c.id AND r.revision=c.effective_revision
 WHERE c.enterprise_id=$1 AND c.id=$2 AND c.status='active' AND c.effective_collector_id IS NOT NULL AND r.credential_id=$3);

-- name: EnqueueMonitoringAssignmentRetirement :exec
INSERT INTO runtime_tasks(id,enterprise_id,queue,payload,max_attempts,available_at)
VALUES($1,$2,'monitoring_assignment_retire',$3,8,now()) ON CONFLICT(id) DO NOTHING;

-- name: CompleteMonitoringAssignmentRetirements :exec
UPDATE monitoring_assignment_retirements SET status='completed',error_code='',updated_at=now()
WHERE enterprise_id=$1 AND previous_collector_id=$2 AND collection_id<>ALL($3::uuid[]) AND status<>'completed';

-- name: ClearRetiredCollectorCollectionExecution :exec
UPDATE monitoring_collections SET effective_collector_id=NULL,effective_assignment_epoch=0,health='stopped',observed_at=now()
WHERE enterprise_id=$1 AND effective_collector_id=$2 AND status='archived' AND id<>ALL($3::uuid[]);

-- name: ListUnusedMonitoringCredentials :many
SELECT cred.* FROM credentials cred JOIN secrets secret ON secret.id=cred.secret_id AND secret.enterprise_id=cred.enterprise_id
WHERE cred.enterprise_id=$1 AND cred.protocol='monitoring' AND cred.status='active'
AND secret.purpose='monitoring' AND secret.owner_type='monitoring_collection' AND secret.owner_id=$2
AND NOT EXISTS(SELECT 1 FROM monitoring_collections c WHERE c.enterprise_id=cred.enterprise_id AND c.id=$2 AND c.credential_id=cred.id AND c.status='active')
AND NOT EXISTS(SELECT 1 FROM monitoring_collections c JOIN monitoring_collection_revisions r ON r.collection_id=c.id AND r.enterprise_id=c.enterprise_id AND r.revision=c.effective_revision
 WHERE c.enterprise_id=cred.enterprise_id AND c.id=$2 AND c.status='active' AND c.effective_collector_id IS NOT NULL AND r.credential_id=cred.id);
