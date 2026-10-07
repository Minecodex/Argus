-- name: GetPlatformOverviewCounts :one
SELECT
  (SELECT count(*) FROM enterprises)::bigint AS enterprise_count,
  (SELECT count(*) FROM enterprises WHERE status = 'active')::bigint AS active_enterprise_count,
  (SELECT count(*) FROM sandbox_sessions WHERE status IN ('creating', 'running', 'terminating', 'unknown'))::bigint AS active_sandbox_session_count,
  (SELECT count(*) FROM enterprise_users eu
   WHERE eu.status = 'active' AND eu.last_login_at IS NULL AND EXISTS (
     SELECT 1 FROM role_bindings rb
     JOIN roles r ON r.id = rb.role_id AND r.enterprise_id = rb.enterprise_id
     WHERE rb.enterprise_id = eu.enterprise_id AND rb.subject_type = 'user' AND rb.subject_id = eu.id
       AND rb.status = 'active' AND r.identity_key = 'enterprise_admin' AND r.builtin AND r.status = 'active'
       AND (rb.valid_from IS NULL OR rb.valid_from <= now()) AND (rb.valid_until IS NULL OR rb.valid_until > now())
   ))::bigint AS pending_admin_count;

-- name: GetPlatformMonthlySandboxUsage :many
SELECT to_char(month, 'YYYY-MM')::text AS month,
  sum(session_count)::bigint AS session_count, sum(session_seconds)::bigint AS session_seconds
FROM sandbox_usage
WHERE month >= sqlc.arg(from_month)::date AND month < sqlc.arg(to_month)::date
GROUP BY month ORDER BY month;
