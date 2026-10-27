-- +goose Up
CREATE VIEW observation_resource_directory AS
SELECT h.enterprise_id,h.id,'host'::text AS kind,h.name,h.environment,h.labels,h.resource_version AS version,
 NULL::uuid AS parent_id,''::text AS parent_name,0::bigint AS parent_version,''::text AS namespace_uid
FROM hosts h WHERE h.status='active'
UNION ALL
SELECT c.enterprise_id,c.id,'kubernetes_cluster'::text,c.name,c.environment,c.labels,c.resource_version,NULL::uuid,'',0::bigint,''
FROM kubernetes_clusters c WHERE c.status='active'
UNION ALL
SELECT a.enterprise_id,a.id,'application'::text,a.name,a.environment,'{}'::jsonb,a.version,NULL::uuid,'',0::bigint,''
FROM monitoring_applications a WHERE a.status='active'
UNION ALL
SELECT t.enterprise_id,t.id,'monitoring_target'::text,t.name,''::text,'{}'::jsonb,t.version,NULL::uuid,'',0::bigint,''
FROM monitoring_targets t WHERE t.status='active'
UNION ALL
SELECT n.enterprise_id,n.id,'kubernetes_namespace'::text,n.name,c.environment,'{}'::jsonb,n.version,c.id,c.name,c.resource_version,n.namespace_uid
FROM kubernetes_namespace_resources n JOIN kubernetes_clusters c ON c.id=n.cluster_id AND c.enterprise_id=n.enterprise_id
WHERE n.status='active' AND c.status='active';

-- +goose Down
DROP VIEW observation_resource_directory;
