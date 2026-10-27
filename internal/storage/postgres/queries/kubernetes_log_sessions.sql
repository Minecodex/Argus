-- name: CreateKubernetesLogSession :one
INSERT INTO kubernetes_log_sessions(id,enterprise_id,cluster_id,namespace_id,namespace_uid,actor_id,http_session_id,authorization_version,cluster_version,binding,ticket_hash,ticket_expires_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: GetKubernetesLogSession :one
SELECT * FROM kubernetes_log_sessions WHERE enterprise_id=$1 AND id=$2;

-- name: AttachKubernetesLogSession :one
UPDATE kubernetes_log_sessions SET status='active',ticket_hash=NULL,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND actor_id=$3 AND http_session_id=$4 AND ticket_hash=$5
AND status='pending' AND ticket_expires_at>now() AND expires_at>now() RETURNING *;

-- name: CloseKubernetesLogSession :one
UPDATE kubernetes_log_sessions SET status='closed',ticket_hash=NULL,binding=binding-'continuity_key',version=version+CASE WHEN status='closed' THEN 0 ELSE 1 END,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND actor_id=$3 AND http_session_id=$4 RETURNING *;

-- name: InvalidateKubernetesLogSession :exec
UPDATE kubernetes_log_sessions SET status='invalidated',ticket_hash=NULL,binding=binding-'continuity_key',version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status IN ('pending','active');

-- name: CountActiveKubernetesLogSessions :one
SELECT count(*) FROM kubernetes_log_sessions WHERE enterprise_id=$1 AND actor_id=$2 AND status IN ('pending','active') AND expires_at>now();

-- name: LockKubernetesLogSessionActor :exec
SELECT id FROM enterprise_users WHERE id=$1 FOR UPDATE;

-- name: ValidateKubernetesLogHTTPSession :one
SELECT id FROM sessions WHERE id=$1 AND enterprise_id=$2 AND user_id=$3 AND audience='enterprise'
AND revoked_at IS NULL AND idle_expires_at>now() AND absolute_expires_at>now();

-- name: RetireKubernetesLogSessions :exec
UPDATE kubernetes_log_sessions SET status='invalidated',ticket_hash=NULL,binding=binding-'continuity_key',version=version+1,updated_at=now()
WHERE status IN ('pending','active') AND (expires_at<=now() OR (status='pending' AND ticket_expires_at<=now()));

-- name: DeleteRetiredKubernetesLogSessions :exec
DELETE FROM kubernetes_log_sessions WHERE expires_at<now()-interval '24 hours' AND status IN ('closed','invalidated');
