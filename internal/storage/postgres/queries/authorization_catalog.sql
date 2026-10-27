-- name: ListDataAuthorizationCatalogPage :many
WITH catalog AS (
 SELECT h.id,h.name,h.environment,NULL::uuid AS parent_id,''::text AS parent_name,''::text AS source_uid
 FROM hosts h WHERE h.enterprise_id=sqlc.arg(enterprise_id) AND h.status<>'deleted' AND sqlc.arg(resource_type)::text='host'
 UNION ALL
 SELECT c.id,c.name,c.environment,NULL::uuid,'','' FROM kubernetes_clusters c
 WHERE c.enterprise_id=sqlc.arg(enterprise_id) AND c.status<>'deleted' AND sqlc.arg(resource_type)::text='kubernetes_cluster'
 UNION ALL
 SELECT d.id,d.name,'',NULL::uuid,'','' FROM dashboards d
 WHERE d.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(resource_type)::text='dashboard'
 UNION ALL
 SELECT a.id,a.name,a.environment,NULL::uuid,'','' FROM monitoring_applications a
 WHERE a.enterprise_id=sqlc.arg(enterprise_id) AND a.status='active' AND sqlc.arg(resource_type)::text='application'
 UNION ALL
 SELECT t.id,t.name,'',NULL::uuid,'','' FROM monitoring_targets t
 WHERE t.enterprise_id=sqlc.arg(enterprise_id) AND t.status='active' AND sqlc.arg(resource_type)::text='monitoring_target'
 UNION ALL
 SELECT n.id,n.name,c.environment,c.id,c.name,n.namespace_uid FROM kubernetes_namespace_resources n
 JOIN kubernetes_clusters c ON c.id=n.cluster_id AND c.enterprise_id=n.enterprise_id
 WHERE n.enterprise_id=sqlc.arg(enterprise_id) AND n.status='active' AND c.status='active' AND sqlc.arg(resource_type)::text='kubernetes_namespace'
)
SELECT * FROM catalog WHERE id>sqlc.arg(after_id)::uuid
AND (sqlc.arg(search)::text='' OR name ILIKE '%'||sqlc.arg(search)||'%' OR parent_name ILIKE '%'||sqlc.arg(search)||'%' OR source_uid=sqlc.arg(search))
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: ListCatalogGrantsForResources :many
SELECT * FROM data_authorization_grants WHERE enterprise_id=sqlc.arg(enterprise_id)
AND subject_type=sqlc.arg(subject_type) AND subject_id=sqlc.arg(subject_id)
AND resource_type=sqlc.arg(resource_type) AND resource_id=ANY(sqlc.arg(resource_ids)::uuid[]) AND status='active';

-- name: ListCatalogSourceRoles :many
SELECT DISTINCT role.id,role.name FROM role_bindings binding
JOIN roles role ON role.id=binding.role_id AND role.enterprise_id=binding.enterprise_id AND role.status='active'
LEFT JOIN enterprise_users subject ON sqlc.arg(subject_type)::text='user' AND subject.id=sqlc.arg(subject_id) AND subject.enterprise_id=binding.enterprise_id
WHERE binding.enterprise_id=sqlc.arg(enterprise_id) AND binding.status='active'
AND (binding.valid_from IS NULL OR binding.valid_from<=now()) AND (binding.valid_until IS NULL OR binding.valid_until>now())
AND ((binding.subject_type=sqlc.arg(subject_type) AND binding.subject_id=sqlc.arg(subject_id))
 OR (sqlc.arg(subject_type)::text='user' AND binding.subject_type='department' AND binding.subject_id=subject.department_id))
ORDER BY role.id;
