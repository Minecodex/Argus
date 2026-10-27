-- name: ObserveKubernetesNamespaceObjectIdentity :exec
INSERT INTO kubernetes_namespace_object_identities(enterprise_id,cluster_id,object_uid,object_type,namespace_name,namespace_uid)
SELECT sqlc.arg(enterprise_id),sqlc.arg(cluster_id),sqlc.arg(object_uid),sqlc.arg(object_type),sqlc.arg(namespace_name),sqlc.arg(namespace_uid)
WHERE EXISTS(SELECT 1 FROM kubernetes_namespace_resources n WHERE n.enterprise_id=sqlc.arg(enterprise_id) AND n.cluster_id=sqlc.arg(cluster_id)
 AND n.name=sqlc.arg(namespace_name) AND n.namespace_uid=sqlc.arg(namespace_uid) AND n.status='active')
ON CONFLICT(cluster_id,object_uid) DO UPDATE SET observed_at=clock_timestamp()
WHERE kubernetes_namespace_object_identities.enterprise_id=EXCLUDED.enterprise_id
 AND kubernetes_namespace_object_identities.namespace_uid=EXCLUDED.namespace_uid
 AND kubernetes_namespace_object_identities.object_type=EXCLUDED.object_type
 AND kubernetes_namespace_object_identities.namespace_name=EXCLUDED.namespace_name;

-- name: ResolveKubernetesMetricObjectNamespace :one
SELECT o.namespace_uid FROM kubernetes_namespace_object_identities o
JOIN kubernetes_namespace_resources n ON n.enterprise_id=o.enterprise_id AND n.cluster_id=o.cluster_id AND n.namespace_uid=o.namespace_uid AND n.name=o.namespace_name AND n.status='active'
WHERE o.enterprise_id=sqlc.arg(enterprise_id) AND o.cluster_id=sqlc.arg(cluster_id)
 AND o.namespace_name=sqlc.arg(namespace_name) AND o.object_type=sqlc.arg(object_type) AND o.object_uid=sqlc.arg(object_uid);

-- name: GetKubernetesMetricObjectIdentity :one
SELECT o.object_type,o.namespace_name,o.namespace_uid,
 EXISTS(SELECT 1 FROM kubernetes_namespace_resources n
  WHERE n.enterprise_id=o.enterprise_id AND n.cluster_id=o.cluster_id
   AND n.namespace_uid=o.namespace_uid AND n.name=o.namespace_name AND n.status='active')::boolean AS namespace_active
FROM kubernetes_namespace_object_identities o
WHERE o.enterprise_id=sqlc.arg(enterprise_id) AND o.cluster_id=sqlc.arg(cluster_id) AND o.object_uid=sqlc.arg(object_uid);
