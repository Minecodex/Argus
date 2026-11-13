-- name: ListDashboardOverviewCandidates :many
SELECT d.* FROM dashboards d
JOIN dashboard_revisions r ON r.id=d.active_revision_id AND r.enterprise_id=d.enterprise_id
WHERE d.enterprise_id=sqlc.arg(enterprise_id) AND d.lifecycle='active'
 AND (d.id=ANY(sqlc.arg(dashboard_ids)::uuid[]) OR EXISTS(
  SELECT 1 FROM managed_dashboard_instances m WHERE m.enterprise_id=d.enterprise_id AND m.dashboard_id=d.id))
 AND jsonb_array_length(r.spec->'panels')>0
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(r.spec->'panels') p
  WHERE NOT ((p->'applicable_resource_types') ? 'kubernetes_cluster'
   OR (p->'applicable_resource_types') ? 'kubernetes_namespace'))
 AND (sqlc.arg(search)::text='' OR position(lower(sqlc.arg(search)) in lower(d.name))>0)
 AND (sqlc.arg(after_id)::uuid='00000000-0000-0000-0000-000000000000' OR d.id>sqlc.arg(after_id))
ORDER BY d.id LIMIT sqlc.arg(page_limit);
