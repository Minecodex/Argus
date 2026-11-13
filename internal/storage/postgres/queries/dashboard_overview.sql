-- name: GetDashboardOverviewBinding :one
SELECT * FROM dashboard_bindings
WHERE enterprise_id=$1 AND resource_type='kubernetes_cluster' AND resource_id=$2 AND purpose='overview';

-- name: PutDashboardOverviewBinding :one
INSERT INTO dashboard_bindings(id,enterprise_id,dashboard_id,resource_type,resource_id,created_by,purpose)
VALUES($1,$2,$3,'kubernetes_cluster',$4,$5,'overview')
ON CONFLICT(enterprise_id,resource_type,resource_id) WHERE purpose='overview'
DO UPDATE SET dashboard_id=EXCLUDED.dashboard_id,version=dashboard_bindings.version+1
RETURNING *;
