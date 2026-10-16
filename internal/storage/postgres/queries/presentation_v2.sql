-- name: SetPresentationReadBinding :exec
UPDATE tool_presentations SET read_operations=$3,result_binding=$4 WHERE id=$1 AND enterprise_id=$2;

-- name: CreatePresentationSnapshot :execrows
INSERT INTO tool_presentation_snapshots(presentation_id,enterprise_id,source_version,data)
VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: GetPresentationSnapshot :one
SELECT * FROM tool_presentation_snapshots WHERE presentation_id=$1 AND enterprise_id=$2;

-- name: ListQueryPresentations :many
SELECT p.* FROM tool_presentations p
WHERE p.enterprise_id=$1 AND p.result_binding->>'kind'='dashboard_query' AND p.result_binding->>'job_id'=sqlc.arg(job_id)::text
ORDER BY p.created_at;

-- name: ResolvePresentationResources :many
SELECT h.id,h.name,'host'::text AS resource_type FROM hosts h WHERE h.enterprise_id=$1 AND h.deleted_at IS NULL AND h.id=ANY(sqlc.arg(ids)::uuid[])
UNION ALL SELECT k.id,k.name,'kubernetes_cluster'::text AS resource_type FROM kubernetes_clusters k WHERE k.enterprise_id=$1 AND k.deleted_at IS NULL AND k.id=ANY(sqlc.arg(ids)::uuid[])
ORDER BY id;

-- name: ListUnsealedPresentationJobs :many
SELECT DISTINCT j.id,j.enterprise_id FROM dashboard_query_jobs j
JOIN tool_presentations p ON p.enterprise_id=j.enterprise_id AND p.result_binding->>'kind'='dashboard_query' AND p.result_binding->>'job_id'=j.id::text
LEFT JOIN tool_presentation_snapshots s ON s.presentation_id=p.id
WHERE s.presentation_id IS NULL AND (j.manifest IS NOT NULL OR j.status IN ('complete','partial','failed','cancelled'))
LIMIT 50;
