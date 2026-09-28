-- name: ListAuditEventsPage :many
SELECT * FROM audit_events
WHERE domain = sqlc.arg(domain)::text
  AND ((domain = 'platform' AND enterprise_id IS NULL)
    OR (domain = 'enterprise' AND enterprise_id = sqlc.narg(enterprise_id)::uuid))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action)::text)
  AND (sqlc.narg(actor_id)::text IS NULL OR actor_id = sqlc.narg(actor_id)::text)
  AND (sqlc.narg(resource_type)::text IS NULL OR resource_type = sqlc.narg(resource_type)::text)
  AND (sqlc.narg(resource_id)::text IS NULL OR resource_id = sqlc.narg(resource_id)::text)
  AND (sqlc.narg(result)::text IS NULL OR result = sqlc.narg(result)::text)
  AND (sqlc.narg(since)::timestamptz IS NULL OR created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR created_at < sqlc.narg(until)::timestamptz)
  AND (sqlc.narg(search)::text IS NULL OR position(lower(btrim(sqlc.narg(search)::text)) IN lower(concat_ws(' ', action, actor_id, resource_type, resource_id, details::text))) > 0)
  AND (sqlc.narg(before_time)::timestamptz IS NULL OR (created_at,id) < (sqlc.narg(before_time)::timestamptz,sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size)::int;
