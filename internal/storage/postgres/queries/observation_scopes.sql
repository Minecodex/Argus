-- name: ListObservationScopeResources :many
SELECT h.id,'host'::text AS kind,h.name,h.environment,h.labels,h.resource_version AS version,
 '00000000-0000-0000-0000-000000000000'::uuid AS parent_id,0::bigint AS parent_version,''::text AS namespace_uid
FROM hosts h WHERE h.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(kind)::text='host'
 AND h.id=ANY(sqlc.arg(ids)::uuid[]) AND h.status='active'
UNION ALL
SELECT k.id,'kubernetes_cluster'::text,k.name,k.environment,k.labels,k.resource_version,
 '00000000-0000-0000-0000-000000000000'::uuid,0::bigint,''::text
FROM kubernetes_clusters k WHERE k.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(kind)::text='kubernetes_cluster'
 AND k.id=ANY(sqlc.arg(ids)::uuid[]) AND k.status='active'
UNION ALL
SELECT a.id,'application'::text,a.name,a.environment,'{}'::jsonb,a.version,
 '00000000-0000-0000-0000-000000000000'::uuid,0::bigint,''::text
FROM monitoring_applications a WHERE a.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(kind)::text='application'
 AND a.id=ANY(sqlc.arg(ids)::uuid[])
UNION ALL
SELECT t.id,'monitoring_target'::text,t.name,''::text,'{}'::jsonb,t.version,
 '00000000-0000-0000-0000-000000000000'::uuid,0::bigint,''::text
FROM monitoring_targets t WHERE t.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(kind)::text='monitoring_target'
 AND t.id=ANY(sqlc.arg(ids)::uuid[])
UNION ALL
SELECT namespace.id,'kubernetes_namespace'::text,cluster.name||' / '||namespace.name,cluster.environment,'{}'::jsonb,namespace.version,
 cluster.id,cluster.resource_version,namespace.namespace_uid
FROM kubernetes_namespace_resources namespace JOIN kubernetes_clusters cluster
 ON cluster.id=namespace.cluster_id AND cluster.enterprise_id=namespace.enterprise_id AND cluster.status='active'
WHERE namespace.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(kind)::text='kubernetes_namespace'
 AND namespace.id=ANY(sqlc.arg(ids)::uuid[]) AND namespace.status='active'
ORDER BY kind,id;

-- name: ListObservationResourceDirectoryPage :many
SELECT * FROM observation_resource_directory
WHERE enterprise_id=sqlc.arg(enterprise_id) AND kind=sqlc.arg(kind) AND id=ANY(sqlc.arg(authorized_ids)::uuid[])
 AND id>sqlc.arg(after_id)::uuid
 AND (sqlc.arg(search)::text='' OR name ILIKE '%'||sqlc.arg(search)||'%' OR parent_name ILIKE '%'||sqlc.arg(search)||'%'
   OR id::text=sqlc.arg(search) OR namespace_uid=sqlc.arg(search))
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: ListObservationSources :many
SELECT source.*,
 (source.generation=collector.source_generation AND collector.status NOT IN ('uninstalled','uninstalling'))::boolean AS current_installation,
 array_agg(DISTINCT scope.id ORDER BY scope.id)::uuid[] AS scope_resource_ids
FROM telemetry_sources source JOIN collector_instances collector
 ON collector.id=source.collector_id AND collector.enterprise_id=source.enterprise_id
JOIN jsonb_to_recordset(sqlc.arg(scope_refs)::jsonb) AS scope(type text,id uuid,cluster_id uuid,namespace_uid text,allow_infrastructure boolean)
 ON (scope.type=source.resource_type AND scope.id=source.resource_id)
 OR (scope.type='kubernetes_namespace' AND scope.allow_infrastructure
  AND scope.cluster_id=source.resource_id AND scope.namespace_uid<>''
  AND source.resource_type='kubernetes_cluster' AND source.collection_id IS NOT NULL
  AND 'metrics'=ANY(source.signals) AND source.source_type IN ('k8s_cluster','kubeletstats'))
 OR (source.collection_id IS NOT NULL AND source.namespace_uid<>'' AND 'logs'=ANY(source.signals)
  AND collector.resource_type='kubernetes_cluster'
  AND ((scope.type='kubernetes_namespace' AND scope.cluster_id=collector.resource_id AND scope.namespace_uid=source.namespace_uid)
    OR (scope.type='kubernetes_cluster' AND scope.id=collector.resource_id))
  AND (scope.type<>'kubernetes_namespace' OR scope.allow_infrastructure OR source.resource_type='application'
    OR (source.resource_type='monitoring_target' AND source.resource_id=ANY(sqlc.arg(selected_target_ids)::uuid[])))
  AND (source.resource_type='kubernetes_cluster' OR sqlc.arg(operational_logs)::boolean
    OR (source.resource_type='application' AND source.resource_id=ANY(sqlc.arg(application_ids)::uuid[]))
    OR (source.resource_type='monitoring_target' AND source.resource_id=ANY(sqlc.arg(target_ids)::uuid[]))))
WHERE source.enterprise_id=sqlc.arg(enterprise_id)
 AND (sqlc.arg(source_type)::text='' OR source.source_type=sqlc.arg(source_type))
 AND (sqlc.arg(signal)::text='' OR sqlc.arg(signal)=ANY(source.signals))
 AND (NOT sqlc.arg(filter_applications)::boolean OR source.resource_type<>'application'
   OR source.resource_id=ANY(sqlc.arg(selected_application_ids)::uuid[]))
GROUP BY source.id,source.config_revision,collector.source_generation,collector.status
ORDER BY source.id,source.config_revision;
