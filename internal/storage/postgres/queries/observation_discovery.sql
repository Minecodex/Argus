-- name: ListObservationDiscoveryPage :many
SELECT d.*, COALESCE(t.engine,'')::text AS integration,
 COALESCE((SELECT array_agg(DISTINCT integration_key ORDER BY integration_key) FROM (
 SELECT c.integration_key FROM monitoring_collections c WHERE c.enterprise_id=d.enterprise_id AND c.target_type=d.kind AND c.target_id=d.id
 UNION SELECT e.protocol AS integration_key FROM monitoring_ingest_endpoints e WHERE e.enterprise_id=d.enterprise_id AND d.kind='application' AND e.application_id=d.id
 UNION SELECT 'sdklog'::text AS integration_key FROM monitoring_ingest_endpoints e WHERE e.enterprise_id=d.enterprise_id AND d.kind='application' AND e.application_id=d.id AND e.protocol='otlp' AND 'logs'=ANY(e.signals)
 ) integrations),ARRAY[]::text[])::text[] AS integrations
FROM observation_resource_directory d
JOIN jsonb_to_recordset(sqlc.arg(authorized_refs)::jsonb) AS allowed(type text,id uuid)
 ON allowed.type=d.kind AND allowed.id=d.id
LEFT JOIN monitoring_targets t ON t.enterprise_id=d.enterprise_id AND d.kind='monitoring_target' AND t.id=d.id
WHERE d.enterprise_id=sqlc.arg(enterprise_id)
 AND (d.kind,d.id)>(sqlc.arg(after_kind)::text,sqlc.arg(after_id)::uuid)
 AND (sqlc.arg(search)::text='' OR d.name ILIKE '%'||sqlc.arg(search)||'%'
 OR d.parent_name ILIKE '%'||sqlc.arg(search)||'%' OR d.id::text=sqlc.arg(search))
 AND (NOT sqlc.arg(filter_integrations)::boolean OR t.engine=ANY(sqlc.arg(integrations)::text[])
 OR EXISTS(SELECT 1 FROM monitoring_collections c WHERE c.enterprise_id=d.enterprise_id
 AND c.target_type=d.kind AND c.target_id=d.id AND c.integration_key=ANY(sqlc.arg(integrations)::text[]))
 OR EXISTS(SELECT 1 FROM monitoring_ingest_endpoints e WHERE e.enterprise_id=d.enterprise_id AND d.kind='application' AND e.application_id=d.id
 AND (e.protocol=ANY(sqlc.arg(integrations)::text[]) OR ('sdklog'=ANY(sqlc.arg(integrations)::text[]) AND e.protocol='otlp' AND 'logs'=ANY(e.signals)))))
ORDER BY d.kind,d.id LIMIT sqlc.arg(page_limit);

-- name: ListObservationCollectionStates :many
SELECT * FROM monitoring_collections
WHERE enterprise_id=sqlc.arg(enterprise_id) AND target_type=sqlc.arg(target_type) AND target_id=sqlc.arg(target_id)
 AND id>sqlc.arg(after_id)::uuid ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: ObservationCollectionRevision :one
SELECT md5(COALESCE(string_agg(id::text||':'||version::text||':'||desired_revision::text||':'||effective_revision::text||':'||COALESCE(collector_id::text,'')||':'||COALESCE(effective_collector_id::text,'')||':'||assignment_epoch::text||':'||effective_assignment_epoch::text,',' ORDER BY id),''))::text
FROM monitoring_collections WHERE enterprise_id=sqlc.arg(enterprise_id)
 AND target_type=sqlc.arg(target_type) AND target_id=sqlc.arg(target_id);
