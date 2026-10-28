-- name: CreateMonitoringTarget :one
INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: GetMonitoringTarget :one
SELECT * FROM monitoring_targets WHERE enterprise_id=$1 AND id=$2;

-- name: LockMonitoringTarget :one
SELECT * FROM monitoring_targets WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ListMonitoringTargets :many
SELECT * FROM monitoring_targets WHERE enterprise_id=sqlc.arg(enterprise_id)
 AND id=ANY(sqlc.arg(authorized_ids)::uuid[]) AND status='active' AND id>sqlc.arg(after_id)::uuid
 AND (sqlc.arg(engine)::text='' OR engine=sqlc.arg(engine))
 AND (sqlc.arg(search)::text='' OR name ILIKE '%'||sqlc.arg(search)||'%')
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: UpdateMonitoringTarget :one
UPDATE monitoring_targets SET name=$4,identity_config=$5,status=$6,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$3 RETURNING *;

-- name: CreateMonitoringCollection :one
INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,integration_version,target_type,target_id,collector_id,application_id,service_id,config,credential_id,desired_enabled,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *;

-- name: GetMonitoringCollection :one
SELECT * FROM monitoring_collections WHERE enterprise_id=$1 AND id=$2;

-- name: LockMonitoringCollection :one
SELECT * FROM monitoring_collections WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ListMonitoringCollections :many
SELECT collection.* FROM monitoring_collections collection WHERE collection.enterprise_id=sqlc.arg(enterprise_id) AND collection.status='active'
 -- Current inventory excludes retired owners, while collection records remain
 -- available to historical/audit reads. Apply this before pagination.
 AND CASE collection.target_type
   WHEN 'host' THEN EXISTS(SELECT 1 FROM hosts owner WHERE owner.enterprise_id=collection.enterprise_id AND owner.id=collection.target_id AND owner.status<>'deleted')
   WHEN 'kubernetes_cluster' THEN EXISTS(SELECT 1 FROM kubernetes_clusters owner WHERE owner.enterprise_id=collection.enterprise_id AND owner.id=collection.target_id AND owner.status<>'deleted')
   WHEN 'monitoring_target' THEN EXISTS(SELECT 1 FROM monitoring_targets owner WHERE owner.enterprise_id=collection.enterprise_id AND owner.id=collection.target_id AND owner.status='active')
   WHEN 'application' THEN EXISTS(SELECT 1 FROM monitoring_applications owner WHERE owner.enterprise_id=collection.enterprise_id AND owner.id=collection.target_id AND owner.status='active')
   ELSE false END
 AND ((collection.target_type='host' AND collection.target_id=ANY(sqlc.arg(host_ids)::uuid[]))
 OR (collection.target_type='kubernetes_cluster' AND collection.target_id=ANY(sqlc.arg(cluster_ids)::uuid[]))
 OR (collection.target_type='monitoring_target' AND collection.target_id=ANY(sqlc.arg(target_ids)::uuid[]))
 OR (collection.target_type='application' AND collection.target_id=ANY(sqlc.arg(application_ids)::uuid[]))
 OR (collection.target_type='kubernetes_cluster' AND collection.integration_key='k8slog' AND EXISTS(
   SELECT 1 FROM kubernetes_namespace_resources namespace JOIN collector_instances collector
     ON collector.enterprise_id=namespace.enterprise_id AND collector.resource_id=namespace.cluster_id AND collector.resource_type='kubernetes_cluster'
   WHERE namespace.enterprise_id=collection.enterprise_id AND namespace.id=ANY(sqlc.arg(namespace_ids)::uuid[])
     AND namespace.status='active' AND collector.id=collection.collector_id
     AND namespace.id::text=collection.config#>>'{workload,namespace_id}'
     AND namespace.namespace_uid=collection.config#>>'{workload,namespace_uid}'
 ))) AND collection.id>sqlc.arg(after_id)::uuid
 AND (sqlc.arg(integration_key)::text='' OR collection.integration_key=sqlc.arg(integration_key))
 AND (sqlc.arg(search)::text='' OR collection.name ILIKE '%'||sqlc.arg(search)||'%')
 AND (sqlc.narg(desired_enabled)::boolean IS NULL OR collection.desired_enabled=sqlc.narg(desired_enabled))
 AND (sqlc.arg(apply_status)::text='' OR collection.apply_status=sqlc.arg(apply_status))
 AND (sqlc.arg(health)::text='' OR collection.health=sqlc.arg(health))
 -- All collection write paths normalize interval with time.Duration.String().
 -- Match collectionDTO's three-interval / two-minute threshold before LIMIT.
 AND (sqlc.arg(freshness)::text='' OR
   CASE WHEN collection.last_sample_at IS NULL THEN 'no_data'
        WHEN collection.last_sample_at < sqlc.arg(observed_at)::timestamptz - GREATEST(
          interval '2 minutes',
          3 * CASE WHEN pg_input_is_valid(NULLIF(collection.config->>'interval',''),'interval')
                   THEN (collection.config->>'interval')::interval ELSE interval '30 seconds' END
        ) THEN 'stale' ELSE 'fresh' END = sqlc.arg(freshness))
ORDER BY collection.id LIMIT sqlc.arg(page_limit);

-- name: ListCollectorMonitoringCollections :many
SELECT * FROM monitoring_collections WHERE enterprise_id=$1 AND (collector_id=$2 OR effective_collector_id=$2) AND status='active' ORDER BY id;

-- name: LockLogCollectionClaims :exec
SELECT pg_advisory_xact_lock(hashtextextended('argus-log-claims:'||sqlc.arg(enterprise_id)::uuid::text||':'||sqlc.arg(resource_type)::text||':'||sqlc.arg(resource_id)::uuid::text,0));

-- name: ListResourceLogCollections :many
SELECT collection.* FROM monitoring_collections collection
JOIN collector_instances collector ON collector.id=collection.collector_id AND collector.enterprise_id=collection.enterprise_id
WHERE collection.enterprise_id=sqlc.arg(enterprise_id) AND collection.status='active' AND collection.desired_enabled
AND collection.integration_key IN ('filelog','journald','eventlog','k8slog')
AND collector.resource_type=sqlc.arg(resource_type) AND collector.resource_id=sqlc.arg(resource_id)
ORDER BY collection.id;

-- name: ListCollectorLegacyLogProfiles :many
SELECT profile.profile_key FROM collector_instances collector
JOIN collector_config_revisions config ON config.collector_id=collector.id AND config.revision=collector.desired_revision
JOIN collection_profiles profile ON profile.id=ANY(config.profile_ids)
WHERE collector.enterprise_id=$1 AND collector.id=$2
AND profile.profile_key IN ('file-log','linux-journald','windows-event-log','k8s-node-container');

-- name: UpdateMonitoringCollection :one
UPDATE monitoring_collections SET name=$4,config=$5,credential_id=$6,desired_enabled=$7,collector_id=$8,
 application_id=$9,service_id=$10,status=$11,
 assignment_epoch=CASE WHEN collector_id IS DISTINCT FROM $8 THEN assignment_epoch+1 ELSE assignment_epoch END,
 desired_revision=desired_revision+1,apply_status='pending',version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$3 RETURNING *;

-- name: SnapshotMonitoringCollection :one
INSERT INTO monitoring_collection_revisions(collection_id,enterprise_id,revision,integration_version,config,desired_enabled,collector_id,credential_id,credential_version,assignment_epoch,created_by)
SELECT c.id,c.enterprise_id,c.desired_revision,c.integration_version,c.config,c.desired_enabled,c.collector_id,c.credential_id,credential.version,c.assignment_epoch,sqlc.arg(actor_id)
FROM monitoring_collections c LEFT JOIN credentials credential ON credential.id=c.credential_id AND credential.enterprise_id=c.enterprise_id
WHERE c.enterprise_id=sqlc.arg(enterprise_id) AND c.id=sqlc.arg(collection_id) RETURNING *;

-- name: EndMonitoringAssignment :exec
UPDATE monitoring_collection_assignments SET valid_to=now() WHERE enterprise_id=$1 AND collection_id=$2 AND valid_to IS NULL;

-- name: CreateMonitoringAssignment :exec
INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id) VALUES($1,$2,$3,$4);

-- name: ObserveMonitoringCollection :execrows
UPDATE monitoring_collections SET effective_revision=$5,apply_status=$6,health=$7,
 last_attempt_at=sqlc.narg(last_attempt_at),last_sample_at=COALESCE(sqlc.narg(last_sample_at),last_sample_at),
 error_code=$8,error_message=$9,observed_at=now(),updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND collector_id=$3 AND desired_revision=$4 AND status='active';

-- name: CountMonitoringTargetDependencies :one
SELECT ((SELECT count(*) FROM monitoring_collections c WHERE c.enterprise_id=$1 AND c.target_type=$2 AND c.target_id=$3 AND c.status='active')
 +(SELECT count(*) FROM monitoring_application_bindings b WHERE b.enterprise_id=$1 AND b.target_type=$2 AND b.target_id=$3 AND b.valid_to IS NULL)
 +(SELECT count(*) FROM dashboard_bindings d WHERE d.enterprise_id=$1 AND d.resource_type=$2 AND d.resource_id=$3))::bigint AS count;

-- name: CreateMonitoringCredentialDraft :one
INSERT INTO monitoring_credential_drafts(id,enterprise_id,editor_type,editor_id,collector_id,secret_id,target_hash,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: LockMonitoringCredentialDraft :one
SELECT * FROM monitoring_credential_drafts WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ConsumeMonitoringCredentialDraft :execrows
UPDATE monitoring_credential_drafts SET consumed_at=now()
WHERE enterprise_id=$1 AND id=$2 AND editor_type=$3 AND editor_id=$4 AND consumed_at IS NULL AND expires_at>now();

-- name: MarkMonitoringManagedSecret :exec
UPDATE secrets SET owner_type=$3,owner_id=$4,purpose='monitoring',updated_at=now()
WHERE enterprise_id=$1 AND id=$2;

-- name: RetireMonitoringCredential :exec
UPDATE credentials SET status='disabled',version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND protocol='monitoring';

-- name: GetMonitoringCollectionForTarget :one
SELECT * FROM monitoring_collections WHERE enterprise_id=$1 AND target_type=$2 AND target_id=$3 AND integration_key=$4 AND status='active';

-- name: GetMonitoringCollectionRevision :one
SELECT * FROM monitoring_collection_revisions WHERE enterprise_id=$1 AND collection_id=$2 AND revision=$3;

-- name: GetMonitoringDistribution :one
SELECT * FROM collector_distribution_versions WHERE id=$1;

-- name: CreateMonitoringCredentialDelivery :exec
INSERT INTO monitoring_credential_deliveries(lease_id,enterprise_id,collection_id,collection_revision,assignment_epoch,collector_id,collector_revision)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: GetMonitoringCredentialDelivery :one
SELECT * FROM monitoring_credential_deliveries WHERE enterprise_id=$1 AND lease_id=$2;

-- name: GetLatestCollectorManagementPayload :one
SELECT payload FROM (
 SELECT command.payload,command.created_at FROM connector_commands command WHERE command.enterprise_id=sqlc.arg(enterprise_id) AND command.command_type='collector_management' AND command.payload->>'collector_id'=sqlc.arg(collector_id)::text
 UNION ALL
 SELECT operation.plan AS payload,operation.created_at FROM telemetry_collector_operations operation WHERE operation.enterprise_id=sqlc.arg(enterprise_id) AND operation.collector_id::text=sqlc.arg(collector_id)::text
) operations ORDER BY created_at DESC LIMIT 1;

-- name: LockExpiredMonitoringCredentialDrafts :many
SELECT d.* FROM monitoring_credential_drafts d JOIN secrets s ON s.id=d.secret_id AND s.enterprise_id=d.enterprise_id
WHERE d.consumed_at IS NULL AND s.owner_type='monitoring_draft' AND s.owner_id=d.id AND s.purpose='monitoring'
 AND (d.expires_at<=now() OR EXISTS(
  SELECT 1 FROM pending_action_plans p JOIN pending_actions a ON a.id=p.pending_action_id AND a.enterprise_id=p.enterprise_id
  WHERE p.enterprise_id=d.enterprise_id AND p.immutable_plan->'mutation'->'collection'->>'credential_draft_id'=d.id::text
   AND a.status IN ('cancelled','expired','rejected','invalidated','failed')))
 AND NOT EXISTS(SELECT 1 FROM credentials c WHERE c.secret_id=s.id AND c.enterprise_id=s.enterprise_id)
ORDER BY d.expires_at,d.id LIMIT $1 FOR UPDATE OF d,s SKIP LOCKED;

-- name: DeleteUnusedMonitoringCredentialDraft :execrows
DELETE FROM monitoring_credential_drafts d USING secrets s
WHERE d.enterprise_id=$1 AND d.id=$2 AND d.consumed_at IS NULL AND s.id=d.secret_id AND s.enterprise_id=d.enterprise_id
 AND s.owner_type='monitoring_draft' AND s.owner_id=d.id AND s.purpose='monitoring'
 AND NOT EXISTS(SELECT 1 FROM credentials c WHERE c.secret_id=s.id AND c.enterprise_id=s.enterprise_id);

-- name: DeleteUnusedMonitoringDraftVersions :exec
DELETE FROM secret_versions v USING secrets s
WHERE s.enterprise_id=$1 AND s.id=$2 AND v.secret_id=s.id AND v.enterprise_id=s.enterprise_id
 AND s.owner_type='monitoring_draft' AND s.owner_id=$3 AND s.purpose='monitoring'
 AND NOT EXISTS(SELECT 1 FROM credentials c WHERE c.secret_id=s.id AND c.enterprise_id=s.enterprise_id);

-- name: DeleteUnusedMonitoringDraftSecret :execrows
DELETE FROM secrets s WHERE s.enterprise_id=$1 AND s.id=$2 AND s.owner_type='monitoring_draft' AND s.owner_id=$3 AND s.purpose='monitoring'
 AND NOT EXISTS(SELECT 1 FROM credentials c WHERE c.secret_id=s.id AND c.enterprise_id=s.enterprise_id);

-- name: ApplyCollectorRevisionOutcome :execrows
UPDATE collector_instances SET status=sqlc.arg(status),
 effective_revision=CASE WHEN sqlc.arg(applied)::boolean THEN sqlc.arg(revision)::bigint ELSE effective_revision END,
 version=version+CASE WHEN status IS DISTINCT FROM sqlc.arg(status)
   OR (sqlc.arg(applied)::boolean AND effective_revision IS DISTINCT FROM sqlc.arg(revision)::bigint) THEN 1 ELSE 0 END,
 updated_at=CASE WHEN status IS DISTINCT FROM sqlc.arg(status)
   OR (sqlc.arg(applied)::boolean AND effective_revision IS DISTINCT FROM sqlc.arg(revision)::bigint) THEN now() ELSE updated_at END
WHERE enterprise_id=sqlc.arg(enterprise_id) AND id=sqlc.arg(collector_id) AND desired_revision=sqlc.arg(revision)
 AND (sqlc.arg(applied)::boolean OR effective_revision<sqlc.arg(revision));

-- name: ApplyMonitoringCollectionRevision :execrows
UPDATE monitoring_collections SET
 effective_revision=CASE WHEN sqlc.arg(applied)::boolean THEN sqlc.arg(revision)::bigint ELSE effective_revision END,
 effective_collector_id=CASE WHEN sqlc.arg(applied)::boolean THEN collector_id ELSE effective_collector_id END,
 effective_assignment_epoch=CASE WHEN sqlc.arg(applied)::boolean THEN assignment_epoch ELSE effective_assignment_epoch END,
 apply_status=CASE WHEN sqlc.arg(applied)::boolean THEN 'effective' WHEN sqlc.arg(outcome_unknown)::boolean THEN 'unknown' ELSE 'failed' END,
 health=CASE WHEN sqlc.arg(applied)::boolean AND NOT desired_enabled THEN 'stopped' ELSE 'unknown' END,
 last_attempt_at=now(),observed_at=now(),error_code=sqlc.arg(error_code),error_message='',updated_at=now()
WHERE enterprise_id=sqlc.arg(enterprise_id) AND id=sqlc.arg(collection_id) AND collector_id=sqlc.arg(collector_id)
 AND desired_revision=sqlc.arg(revision) AND assignment_epoch=sqlc.arg(assignment_epoch) AND status='active';

-- name: ObserveMonitoringSamples :execrows
UPDATE monitoring_collections c SET
 last_sample_at=CASE WHEN sample.has_data IS DISTINCT FROM false THEN GREATEST(c.last_sample_at,COALESCE(sample.data_at,now())) ELSE c.last_sample_at END,
 last_attempt_at=COALESCE(sample.observed_at,now()),observed_at=now(),health=COALESCE(sample.health,'healthy'),
 error_code=CASE WHEN c.error_code IN ('','COLLECTION_SCRAPE_FAILED','COLLECTION_SCRAPE_PARTIAL_FAILURE') THEN COALESCE(sample.error_code,'') ELSE c.error_code END
FROM jsonb_to_recordset(sqlc.arg(samples)::jsonb) AS sample(collection_id uuid,revision bigint,epoch bigint,health text,error_code text,has_data boolean,observed_at timestamptz,data_at timestamptz)
WHERE c.enterprise_id=sqlc.arg(enterprise_id) AND c.effective_collector_id=sqlc.arg(collector_id)
 AND c.id=sample.collection_id AND c.effective_revision=sample.revision AND c.effective_assignment_epoch=sample.epoch
 AND c.status='active' AND c.desired_enabled
 AND (sample.observed_at IS NULL OR c.last_attempt_at IS NULL OR sample.observed_at>=c.last_attempt_at);

-- name: ListMonitoringCollectorChoices :many
SELECT c.*,COALESCE(h.name,k.name,'')::text AS resource_name,d.components,d.collector_version,d.version AS distribution_version,d.support_status
FROM collector_instances c JOIN collector_distribution_versions d ON d.id=c.distribution_version_id
LEFT JOIN hosts h ON c.resource_type='host' AND h.id=c.resource_id AND h.enterprise_id=c.enterprise_id
LEFT JOIN kubernetes_clusters k ON c.resource_type='kubernetes_cluster' AND k.id=c.resource_id AND k.enterprise_id=c.enterprise_id
WHERE c.enterprise_id=sqlc.arg(enterprise_id) AND c.id>sqlc.arg(after_id)::uuid
 AND ((c.resource_type='host' AND c.resource_id=ANY(sqlc.arg(host_ids)::uuid[]) AND h.status='active')
  OR (c.resource_type='kubernetes_cluster' AND c.resource_id=ANY(sqlc.arg(cluster_ids)::uuid[]) AND k.status='active'))
 AND c.status NOT IN ('uninstalled','uninstalling')
 AND (sqlc.arg(search)::text='' OR COALESCE(h.name,k.name,'') ILIKE '%'||sqlc.arg(search)||'%')
ORDER BY c.id LIMIT sqlc.arg(page_limit);
