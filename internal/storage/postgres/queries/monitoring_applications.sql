-- name: CreateMonitoringApplication :one
INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,description,owner_name,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: GetMonitoringApplication :one
SELECT * FROM monitoring_applications WHERE enterprise_id=$1 AND id=$2;

-- name: LockMonitoringApplication :one
SELECT * FROM monitoring_applications WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ListMonitoringApplications :many
SELECT * FROM monitoring_applications
WHERE enterprise_id=sqlc.arg(enterprise_id) AND id=ANY(sqlc.arg(authorized_ids)::uuid[])
 AND status=sqlc.arg(status) AND id>sqlc.arg(after_id)::uuid
 AND (sqlc.arg(search)::text='' OR name ILIKE '%'||sqlc.arg(search)||'%' OR application_key ILIKE '%'||sqlc.arg(search)||'%')
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: UpdateMonitoringApplication :one
UPDATE monitoring_applications SET name=$4,description=$5,owner_name=$6,status=$7,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$3 RETURNING *;

-- name: DiscoverMonitoringService :one
INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_namespace,service_name,runtime,first_seen_at,last_seen_at)
SELECT sqlc.arg(id),a.enterprise_id,a.id,sqlc.arg(service_namespace),sqlc.arg(service_name),sqlc.arg(runtime),now(),now()
FROM monitoring_applications a WHERE a.enterprise_id=sqlc.arg(enterprise_id) AND a.id=sqlc.arg(application_id) AND a.status='active'
ON CONFLICT(application_id,service_namespace,service_name) DO UPDATE SET first_seen_at=COALESCE(monitoring_application_services.first_seen_at,now()),last_seen_at=now(),
runtime=CASE WHEN monitoring_application_services.runtime='' THEN EXCLUDED.runtime ELSE monitoring_application_services.runtime END
WHERE monitoring_application_services.status='active' RETURNING *;

-- name: DeclareMonitoringService :one
INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_namespace,service_name)
VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: GetMonitoringService :one
SELECT * FROM monitoring_application_services WHERE enterprise_id=$1 AND id=$2;

-- name: ListMonitoringServices :many
SELECT s.* FROM monitoring_application_services s
WHERE s.enterprise_id=sqlc.arg(enterprise_id) AND s.application_id=ANY(sqlc.arg(application_ids)::uuid[])
 AND s.status='active' AND s.id>sqlc.arg(after_id)::uuid
 AND (sqlc.arg(search)::text='' OR s.service_name ILIKE '%'||sqlc.arg(search)||'%' OR s.service_namespace ILIKE '%'||sqlc.arg(search)||'%')
ORDER BY s.id LIMIT sqlc.arg(page_limit);

-- name: CountMonitoringServices :one
SELECT count(*)::bigint FROM monitoring_application_services WHERE enterprise_id=$1 AND application_id=$2 AND status='active';

-- name: GetMonitoringServiceByName :one
SELECT * FROM monitoring_application_services WHERE enterprise_id=$1 AND application_id=$2 AND service_namespace=$3 AND service_name=$4;

-- name: GetMonitoringInstanceByKey :one
SELECT * FROM monitoring_runtime_instances WHERE enterprise_id=$1 AND service_id=$2 AND instance_key=$3;

-- name: CountMonitoringServiceInstances :one
SELECT count(*)::bigint FROM monitoring_runtime_instances WHERE enterprise_id=$1 AND service_id=$2;

-- name: LockMonitoringDiscoveryScope :exec
SELECT pg_advisory_xact_lock(hashtextextended('argus-discovery:'||sqlc.arg(enterprise_id)::uuid::text||':'||sqlc.arg(application_id)::uuid::text,0));

-- name: ObserveMonitoringInstance :one
INSERT INTO monitoring_runtime_instances(id,enterprise_id,service_id,instance_key,placement,last_seen_at)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(service_id,instance_key) DO UPDATE SET
 placement=CASE WHEN EXCLUDED.last_seen_at>=monitoring_runtime_instances.last_seen_at THEN EXCLUDED.placement ELSE monitoring_runtime_instances.placement END,
 last_seen_at=GREATEST(EXCLUDED.last_seen_at,monitoring_runtime_instances.last_seen_at)
RETURNING *;

-- name: ListMonitoringInstances :many
SELECT * FROM monitoring_runtime_instances WHERE enterprise_id=$1 AND service_id=$2 AND id>$3 ORDER BY id LIMIT $4;

-- name: CreateMonitoringBinding :one
INSERT INTO monitoring_application_bindings(id,enterprise_id,application_id,service_id,target_type,target_id)
VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: ListMonitoringBindings :many
SELECT * FROM monitoring_application_bindings WHERE enterprise_id=$1 AND application_id=ANY($2::uuid[]) AND valid_to IS NULL ORDER BY id;

-- name: EndMonitoringBinding :execrows
UPDATE monitoring_application_bindings SET valid_to=now(),version=version+1
WHERE enterprise_id=$1 AND id=$2 AND version=$3 AND valid_to IS NULL;

-- name: GetMonitoringBinding :one
SELECT * FROM monitoring_application_bindings WHERE enterprise_id=$1 AND id=$2;

-- name: CountApplicationCollectionDependencies :one
SELECT count(*)::bigint FROM monitoring_collections
WHERE enterprise_id=$1 AND application_id=$2 AND status='active' AND desired_enabled;

-- name: ListMonitoringGrantResources :many
SELECT a.id,a.name FROM monitoring_applications a WHERE a.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(resource_type)::text='application' AND a.status='active'
UNION ALL SELECT t.id,t.name FROM monitoring_targets t WHERE t.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(resource_type)::text='monitoring_target' AND t.status='active'
UNION ALL SELECT n.id,n.name FROM kubernetes_namespace_resources n WHERE n.enterprise_id=sqlc.arg(enterprise_id) AND sqlc.arg(resource_type)::text='kubernetes_namespace' AND n.status='active'
ORDER BY name,id;

-- name: MonitoringGrantResourceExists :one
SELECT EXISTS(
 SELECT a.id FROM monitoring_applications a WHERE a.enterprise_id=sqlc.arg(enterprise_id) AND a.id=sqlc.arg(id) AND sqlc.arg(resource_type)::text='application' AND a.status='active'
 UNION ALL SELECT t.id FROM monitoring_targets t WHERE t.enterprise_id=sqlc.arg(enterprise_id) AND t.id=sqlc.arg(id) AND sqlc.arg(resource_type)::text='monitoring_target' AND t.status='active'
 UNION ALL SELECT n.id FROM kubernetes_namespace_resources n WHERE n.enterprise_id=sqlc.arg(enterprise_id) AND n.id=sqlc.arg(id) AND sqlc.arg(resource_type)::text='kubernetes_namespace' AND n.status='active'
)::boolean;

-- name: UpsertKubernetesNamespaceResource :one
INSERT INTO kubernetes_namespace_resources(id,enterprise_id,cluster_id,namespace_uid,name,resource_version,observed_at)
VALUES($1,$2,$3,$4,$5,$6,clock_timestamp())
ON CONFLICT(cluster_id,namespace_uid) DO UPDATE SET name=EXCLUDED.name,resource_version=CASE WHEN EXCLUDED.resource_version<>'' THEN EXCLUDED.resource_version ELSE kubernetes_namespace_resources.resource_version END,
 status='active',observed_at=clock_timestamp(),version=kubernetes_namespace_resources.version+
 CASE WHEN kubernetes_namespace_resources.name<>EXCLUDED.name OR kubernetes_namespace_resources.status<>'active' THEN 1 ELSE 0 END RETURNING *;

-- name: RetireReplacedKubernetesNamespace :exec
UPDATE kubernetes_namespace_resources SET status='deleted',version=version+1,observed_at=clock_timestamp()
WHERE enterprise_id=$1 AND cluster_id=$2 AND name=$3 AND namespace_uid<>$4 AND status='active';

-- name: GetKubernetesNamespaceResource :one
SELECT * FROM kubernetes_namespace_resources WHERE enterprise_id=$1 AND id=$2;

-- name: ResolveKubernetesNamespaceResource :one
SELECT * FROM kubernetes_namespace_resources WHERE enterprise_id=$1 AND cluster_id=$2 AND name=$3 AND status='active';

-- name: ListAuthorizedKubernetesNamespaceResources :many
SELECT * FROM kubernetes_namespace_resources WHERE enterprise_id=sqlc.arg(enterprise_id) AND status='active'
 AND (cluster_id=ANY(sqlc.arg(cluster_ids)::uuid[]) OR id=ANY(sqlc.arg(namespace_ids)::uuid[])) ORDER BY cluster_id,name,id;

-- name: ListNamespaceGrantCatalog :many
SELECT n.*,c.name AS cluster_name FROM kubernetes_namespace_resources n
JOIN kubernetes_clusters c ON c.id=n.cluster_id AND c.enterprise_id=n.enterprise_id
WHERE n.enterprise_id=$1 AND n.status='active' AND c.status='active' ORDER BY c.name,n.name,n.id;
-- name: ObserveMonitoringApplicationService :exec
UPDATE monitoring_application_services service SET first_seen_at=COALESCE(service.first_seen_at,now()),last_seen_at=now()
FROM monitoring_applications application
WHERE service.enterprise_id=$1 AND service.id=$2 AND service.application_id=$3 AND service.status='active'
AND application.enterprise_id=service.enterprise_id AND application.id=service.application_id AND application.status='active';
