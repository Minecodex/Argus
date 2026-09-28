-- name: RegisterTelemetrySource :exec
INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: ResolveTelemetrySource :one
SELECT source.* FROM telemetry_sources source
JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id AND collector.source_generation=source.generation
WHERE source.id=$1 AND source.config_revision=$2 AND source.collector_id=$3 AND source.enterprise_id=$4
 AND source.resource_id=collector.resource_id AND source.resource_type=collector.resource_type
 AND collector.status NOT IN ('uninstalled','uninstalling');

-- name: ListTelemetrySources :many
SELECT source.*, (source.generation=collector.source_generation AND collector.status NOT IN ('uninstalled','uninstalling'))::boolean AS current_installation
FROM telemetry_sources source JOIN collector_instances collector ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id
WHERE source.enterprise_id=sqlc.arg(enterprise_id)
 AND source.resource_id=ANY(sqlc.arg(resource_ids)::uuid[]) AND source.source_type=sqlc.arg(source_type)
 AND sqlc.arg(signal)::text=ANY(source.signals)
ORDER BY source.id,source.config_revision;
