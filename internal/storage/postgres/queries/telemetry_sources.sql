-- name: RegisterTelemetrySource :exec
INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: ResolveTelemetrySource :one
SELECT source.* FROM telemetry_sources source
JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id AND collector.source_generation=source.generation
WHERE source.id=$1 AND source.config_revision=$2 AND source.collector_id=$3 AND source.enterprise_id=$4
 AND ((source.collection_id IS NULL AND source.ingest_endpoint_id IS NULL AND source.resource_id=collector.resource_id AND source.resource_type=collector.resource_type AND collector.effective_revision=source.config_revision)
 OR EXISTS(SELECT 1 FROM monitoring_ingest_endpoints e WHERE e.id=source.ingest_endpoint_id AND e.enterprise_id=source.enterprise_id
 AND e.collector_id=collector.id AND e.application_id=source.resource_id AND source.resource_type='application' AND e.enabled AND e.version=source.ingest_endpoint_version
 AND collector.effective_revision=source.config_revision)
 OR EXISTS(SELECT 1 FROM monitoring_collections c JOIN monitoring_collection_assignments a ON a.collection_id=c.id AND a.enterprise_id=c.enterprise_id
 WHERE c.id=source.collection_id AND c.enterprise_id=source.enterprise_id AND c.effective_collector_id=source.collector_id
 AND c.target_id=source.resource_id AND c.target_type=source.resource_type AND c.status='active' AND c.desired_enabled
 AND c.effective_assignment_epoch=source.assignment_epoch AND a.epoch=c.effective_assignment_epoch AND a.collector_id=c.effective_collector_id AND a.valid_to IS NULL AND a.activated_at IS NOT NULL
 AND c.effective_revision=source.collection_revision))
 AND collector.status NOT IN ('uninstalled','uninstalling');

-- name: TelemetrySourceAwaitingApply :one
SELECT EXISTS(SELECT 1 FROM telemetry_sources source
 JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id AND collector.source_generation=source.generation
 JOIN telemetry_ingest_config_states config ON config.collector_id=collector.id AND config.revision=source.config_revision
 WHERE source.id=sqlc.arg(id) AND source.config_revision=sqlc.arg(config_revision)
 AND source.collector_id=sqlc.arg(collector_id) AND source.enterprise_id=sqlc.arg(enterprise_id)
 AND sqlc.arg(signal)::text=ANY(source.signals)
 AND collector.desired_revision=source.config_revision AND config.status IN ('prepared','applying')
 AND collector.status NOT IN ('uninstalled','uninstalling')
 AND source.source_type<>'skywalking'
 AND (source.source_type NOT IN ('otlp','jaeger') OR source.ingest_endpoint_id IS NOT NULL)
 AND ((source.collection_id IS NULL AND source.ingest_endpoint_id IS NULL AND source.resource_id=collector.resource_id AND source.resource_type=collector.resource_type)
 OR EXISTS(SELECT 1 FROM monitoring_ingest_endpoints e WHERE e.id=source.ingest_endpoint_id AND e.enterprise_id=source.enterprise_id
  AND e.collector_id=collector.id AND e.application_id=source.resource_id AND source.resource_type='application'
  AND e.enabled AND e.version=source.ingest_endpoint_version)
 OR EXISTS(SELECT 1 FROM monitoring_collections c JOIN monitoring_collection_assignments a ON a.collection_id=c.id AND a.enterprise_id=c.enterprise_id
  WHERE c.id=source.collection_id AND c.enterprise_id=source.enterprise_id AND c.collector_id=source.collector_id
  AND c.target_id=source.resource_id AND c.target_type=source.resource_type AND c.status='active' AND c.desired_enabled
  AND c.assignment_epoch=source.assignment_epoch AND c.desired_revision=source.collection_revision
  AND a.epoch=c.assignment_epoch AND a.collector_id=c.collector_id AND a.valid_to IS NULL)));

-- name: RegisterCollectionTelemetrySource :exec
INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch,application_service_id,namespace_uid)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17);

-- name: RegisterApplicationTelemetrySource :exec
INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,ingest_endpoint_id,ingest_endpoint_version)
VALUES($1,$2,$3,$4,'application',$5,$6,$7,$8,$9,$10,$11,$12,$13);

-- name: ListTelemetrySources :many
SELECT source.*, (source.generation=collector.source_generation AND collector.status NOT IN ('uninstalled','uninstalling'))::boolean AS current_installation
FROM telemetry_sources source JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id
WHERE source.enterprise_id=sqlc.arg(enterprise_id)
 AND source.resource_id=ANY(sqlc.arg(resource_ids)::uuid[]) AND source.source_type=sqlc.arg(source_type)
 AND sqlc.arg(signal)::text=ANY(source.signals)
ORDER BY source.id,source.config_revision;

-- name: ListTelemetrySourcesForScope :many
SELECT source.*, (source.generation=collector.source_generation AND collector.status NOT IN ('uninstalled','uninstalling'))::boolean AS current_installation
FROM telemetry_sources source JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id
JOIN jsonb_to_recordset(sqlc.arg(resource_refs)::jsonb) AS resource(type text,id uuid)
 ON resource.id=source.resource_id AND resource.type=source.resource_type
WHERE source.enterprise_id=sqlc.arg(enterprise_id)
 AND (sqlc.arg(source_type)::text='' OR source.source_type=sqlc.arg(source_type))
 AND (sqlc.arg(signal)::text='' OR sqlc.arg(signal)=ANY(source.signals))
ORDER BY source.id,source.config_revision;
