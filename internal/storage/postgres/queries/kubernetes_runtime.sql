-- name: UpdateKubernetesRuntimeObservation :execrows
UPDATE kubernetes_clusters cluster
SET node_count=$4,ready_node_count=$5,kubernetes_version=$6,statistics_observed_at=$7
FROM connectors connector
WHERE cluster.connector_id=$1 AND cluster.enterprise_id=$2 AND cluster.status='active'
AND connector.id=cluster.connector_id AND connector.enterprise_id=cluster.enterprise_id
AND connector.role='kubernetes' AND connector.connection_epoch=$3 AND connector.status='online'
AND (cluster.statistics_observed_at IS NULL OR cluster.statistics_observed_at<=$7);
