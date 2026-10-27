-- name: ListAlertConfigurations :many
SELECT * FROM alert_configurations WHERE enterprise_id=$1 AND kind=$2 AND status='active' ORDER BY updated_at DESC,id;

-- name: GetAlertConfiguration :one
SELECT * FROM alert_configurations WHERE enterprise_id=$1 AND id=$2 AND status='active';

-- name: LockAlertConfiguration :one
SELECT * FROM alert_configurations WHERE enterprise_id=$1 AND id=$2 AND status='active' FOR UPDATE;

-- name: CreateAlertConfiguration :one
INSERT INTO alert_configurations(id,enterprise_id,kind,name,spec,created_by)
VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: UpdateAlertConfiguration :one
UPDATE alert_configurations SET name=$3,spec=$4,version=version+1,generation=$5,updated_at=now(),next_evaluation_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$6 AND status='active' RETURNING *;

-- name: DeleteAlertConfiguration :one
UPDATE alert_configurations SET status='deleted',version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$3 AND status='active' RETURNING *;

-- name: AddAlertConfigurationRevision :exec
INSERT INTO alert_configuration_revisions(configuration_id,enterprise_id,version,name,spec,actor_id) VALUES($1,$2,$3,$4,$5,$6);

-- name: ListAlertConfigurationRevisions :many
SELECT * FROM alert_configuration_revisions WHERE enterprise_id=$1 AND configuration_id=$2 ORDER BY version DESC;

-- name: CountAlertNoiseHits :one
SELECT count(*) FROM alert_instances WHERE enterprise_id=$1 AND resource_ids <@ $2::uuid[]
AND lifecycle IN ('firing','recovering') AND EXISTS(
SELECT 1 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(state->'noise_reasons')='array' THEN state->'noise_reasons' ELSE '[]'::jsonb END) reason
WHERE reason LIKE $3::text);

-- name: DueAlertPolicies :many
SELECT * FROM alert_configurations WHERE kind='policy' AND status='active' AND next_evaluation_at<=now()
AND COALESCE((spec->>'enabled')::boolean,false) ORDER BY next_evaluation_at,id LIMIT 100 FOR UPDATE SKIP LOCKED;

-- name: SetAlertNextEvaluation :exec
UPDATE alert_configurations SET next_evaluation_at=$3 WHERE enterprise_id=$1 AND id=$2;

-- name: SetAlertEvaluationHealth :exec
UPDATE alert_configurations SET evaluation_health=$3,evaluation_error=$4,last_evaluated_at=$5,last_successful_at=COALESCE($6,last_successful_at)
WHERE enterprise_id=$1 AND id=$2 AND version=$7 AND status='active';

-- name: ListAlertInstances :many
SELECT * FROM alert_instances WHERE enterprise_id=$1 ORDER BY updated_at DESC,id;

-- name: ListPolicyAlertInstances :many
SELECT * FROM alert_instances WHERE enterprise_id=$1 AND policy_id=$2 AND lifecycle NOT IN ('resolved','closed') ORDER BY id;

-- name: GetAlertInstance :one
SELECT * FROM alert_instances WHERE enterprise_id=$1 AND id=$2;

-- name: LockAlertInstance :one
SELECT * FROM alert_instances WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: GetActiveAlertFingerprint :one
SELECT * FROM alert_instances WHERE enterprise_id=$1 AND policy_id=$2 AND fingerprint=$3 AND lifecycle NOT IN ('resolved','closed') FOR UPDATE;

-- name: CreateAlertInstance :one
INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,last_evaluated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *;

-- name: UpdateAlertInstance :one
UPDATE alert_instances SET state=$3,lifecycle=$4,health=$5,severity=$6,started_at=$7,resolved_at=$8,last_evaluated_at=$9,policy_version=COALESCE(sqlc.narg(policy_version),policy_version),version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$10 RETURNING *;

-- name: HandleAlertInstance :one
UPDATE alert_instances SET acknowledged=$3,owner_user_id=$4,handling_version=handling_version+1,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND handling_version=sqlc.arg(expected_handling_version) RETURNING *;

-- name: AddAlertEvent :one
INSERT INTO alert_events(id,enterprise_id,instance_id,policy_id,event_type,resource_ids,payload,actor_id,origin,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: ListAlertEvents :many
SELECT * FROM alert_events WHERE enterprise_id=$1 AND instance_id=$2 ORDER BY created_at,id;

-- name: GetAlertEvent :one
SELECT * FROM alert_events WHERE enterprise_id=$1 AND id=$2;

-- name: CreateAlertDelivery :one
INSERT INTO alert_deliveries(id,enterprise_id,event_id,instance_id,channel_id,channel_version,route_version,dedup_key,group_key,resource_ids,payload,status,available_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT(enterprise_id,dedup_key) DO UPDATE SET dedup_key=EXCLUDED.dedup_key RETURNING *;

-- name: ListAlertDeliveries :many
SELECT * FROM alert_deliveries WHERE enterprise_id=$1 ORDER BY updated_at DESC,id;

-- name: GetAlertDelivery :one
SELECT * FROM alert_deliveries WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: UpdateAlertDelivery :one
UPDATE alert_deliveries SET status=$3,attempt=$4,last_error=$5,last_attempt_at=$6,available_at=$7,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$8 RETURNING *;

-- name: AddAlertDeliveryAttempt :exec
INSERT INTO alert_delivery_attempts(id,enterprise_id,delivery_id,attempt,status,result_code,error_code,started_at,finished_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: ListAlertDeliveryAttempts :many
SELECT * FROM alert_delivery_attempts WHERE enterprise_id=$1 AND delivery_id=$2 ORDER BY attempt;

-- name: AlertRuntimeTaskOwned :one
SELECT EXISTS(SELECT 1 FROM runtime_tasks WHERE id=$1 AND lease_owner=$2 AND fence_token=$3 AND status='running' AND lease_until>now() FOR SHARE)::boolean;

-- name: AlertNotificationTaskOwned :one
SELECT EXISTS(SELECT 1 FROM runtime_tasks WHERE id=$1 AND lease_owner=$2 AND fence_token=$3 AND status='running' AND lease_until>now()
AND queue='alert-notification' AND payload->>'delivery_id'=$4::text AND enterprise_id=$5 FOR SHARE)::boolean;

-- name: LockAlertDeliveryGroup :many
SELECT * FROM alert_deliveries WHERE enterprise_id=$1 AND channel_id=$2 AND group_key=$3 AND status='pending' AND available_at<=now()
ORDER BY created_at,id LIMIT 100 FOR UPDATE SKIP LOCKED;

-- name: AlertMetricSources :many
SELECT source.id,source.resource_type,source.resource_id,source.source_key,source.config_revision,source.config_hash,source.generation
FROM telemetry_sources source JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id
JOIN jsonb_to_recordset(sqlc.arg(resource_refs)::jsonb) AS resource(type text,id uuid) ON resource.id=source.resource_id AND resource.type=source.resource_type
WHERE source.enterprise_id=sqlc.arg(enterprise_id)
AND source.generation=collector.source_generation AND collector.status NOT IN ('uninstalled','uninstalling') AND 'metrics'=ANY(source.signals)
AND source.config_revision=collector.effective_revision
ORDER BY source.id,source.config_revision;

-- name: AddAlertEvaluation :exec
INSERT INTO alert_evaluations(id,enterprise_id,policy_id,policy_version,slot,evaluator_id,authorization_version,scope,health,error_code,fence,result)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(enterprise_id,policy_id,slot) DO NOTHING;

-- name: CreateAlertDeliveryBatch :one
INSERT INTO alert_delivery_batches(id,enterprise_id,channel_id,channel_version,group_key,owner_task_id,fence,payload,status)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,'prepared') RETURNING *;

-- name: GetAlertDeliveryBatch :one
SELECT * FROM alert_delivery_batches WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ListTaskAlertBatches :many
SELECT * FROM alert_delivery_batches WHERE enterprise_id=$1 AND owner_task_id=$2 AND status IN ('prepared','sending') FOR UPDATE;

-- name: AttachAlertDeliveryBatch :exec
UPDATE alert_deliveries SET batch_id=$3 WHERE enterprise_id=$1 AND id=$2 AND status='sending';

-- name: ListAlertBatchDeliveries :many
SELECT * FROM alert_deliveries WHERE enterprise_id=$1 AND batch_id=$2 ORDER BY id FOR UPDATE;

-- name: SetAlertBatchStatus :exec
UPDATE alert_delivery_batches SET status=$3,send_started_at=CASE WHEN $3='sending' THEN now() ELSE send_started_at END,
finished_at=CASE WHEN $3 IN ('accepted','failed','unknown','canceled') THEN now() ELSE finished_at END WHERE enterprise_id=$1 AND id=$2;

-- name: RuntimeTaskLeaseLive :one
SELECT EXISTS(SELECT 1 FROM runtime_tasks WHERE id=$1 AND status IN ('leased','running') AND lease_until>now())::boolean;

-- name: AlertGroupNextSend :one
INSERT INTO alert_notification_groups(enterprise_id,channel_id,group_key,next_send_at) VALUES($1,$2,$3,$4)
ON CONFLICT(enterprise_id,channel_id,group_key) DO UPDATE SET group_key=EXCLUDED.group_key RETURNING *;

-- name: SetAlertGroupSent :exec
UPDATE alert_notification_groups SET last_sent_at=now(),next_send_at=$4 WHERE enterprise_id=$1 AND channel_id=$2 AND group_key=$3;

-- name: SetAlertDeliveryResend :exec
UPDATE alert_deliveries SET resend_of_id=$3 WHERE enterprise_id=$1 AND id=$2;

-- name: ClaimAlertChannelRate :one
INSERT INTO alert_channel_rate_limits(enterprise_id,channel_id,not_before) VALUES($1,$2,$3)
ON CONFLICT(channel_id) DO UPDATE SET not_before=EXCLUDED.not_before WHERE alert_channel_rate_limits.not_before<=now() RETURNING *;

-- name: ListAlertConfiguredResourceIDs :many
-- Configuration visibility includes the grants of a stopped evaluator. This is
-- never an execution authorization: the caller must cover the complete scope.
SELECT DISTINCT dag.resource_id FROM data_authorization_grants dag
JOIN service_accounts subject ON subject.id=sqlc.arg(evaluator_id) AND subject.enterprise_id=dag.enterprise_id
WHERE dag.enterprise_id=sqlc.arg(enterprise_id) AND dag.resource_type=sqlc.arg(resource_type) AND dag.status='active'
AND ((dag.subject_type='service_account' AND dag.subject_id=subject.id) OR (dag.subject_type='role' AND EXISTS(
 SELECT 1 FROM role_bindings binding JOIN roles role ON role.id=binding.role_id AND role.enterprise_id=binding.enterprise_id AND role.status='active'
 WHERE binding.enterprise_id=dag.enterprise_id AND binding.role_id=dag.subject_id AND binding.subject_type='service_account'
 AND binding.subject_id=subject.id AND binding.status='active'
 AND (binding.valid_from IS NULL OR binding.valid_from<=now()) AND (binding.valid_until IS NULL OR binding.valid_until>now())
))) ORDER BY dag.resource_id;

-- name: AlertResourcesStillExist :one
SELECT ((SELECT count(*) FROM (
 SELECT h.id FROM hosts h WHERE h.enterprise_id=sqlc.arg(enterprise_id) AND h.id=ANY(sqlc.arg(resource_ids)::uuid[]) AND h.status<>'deleted'
 UNION
 SELECT k.id FROM kubernetes_clusters k WHERE k.enterprise_id=sqlc.arg(enterprise_id) AND k.id=ANY(sqlc.arg(resource_ids)::uuid[]) AND k.status<>'deleted'
) resources)=cardinality(sqlc.arg(resource_ids)::uuid[]))::boolean;
