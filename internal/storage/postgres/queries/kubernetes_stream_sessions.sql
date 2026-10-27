-- name: CreateKubernetesStreamSession :one
INSERT INTO kubernetes_stream_sessions(id,enterprise_id,cluster_id,namespace_id,namespace_uid,actor_id,http_session_id,authorization_version,cluster_version,mode,binding,ticket_hash,ticket_expires_at,expires_at,connector_id,connector_epoch)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING *;

-- name: GetKubernetesStreamSession :one
SELECT * FROM kubernetes_stream_sessions WHERE id=$1;

-- name: CountActiveKubernetesStreamSessions :one
SELECT count(*) FROM kubernetes_stream_sessions WHERE enterprise_id=$1 AND actor_id=$2 AND status IN ('pending','connecting','active','closing') AND expires_at>now();

-- name: ReplaceKubernetesStreamTicket :one
UPDATE kubernetes_stream_sessions SET ticket_hash=$3,ticket_expires_at=$4,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status='pending' AND expires_at>now() RETURNING *;

-- name: ClaimKubernetesStreamSession :one
UPDATE kubernetes_stream_sessions SET status='connecting',ticket_hash=NULL,owner_gateway=$3,version=version+1,updated_at=now()
WHERE id=$1 AND ticket_hash=$2 AND status='pending' AND ticket_expires_at>now() AND expires_at>now() RETURNING *;

-- name: BindKubernetesStreamLease :one
UPDATE kubernetes_stream_sessions SET credential_lease_id=$4,expires_at=LEAST(expires_at,$5),updated_at=now()
WHERE id=$1 AND fence=$2 AND owner_gateway=$3 AND status='connecting' RETURNING *;

-- name: ActivateKubernetesStreamSession :one
UPDATE kubernetes_stream_sessions SET status='active',connected_at=now(),version=version+1,updated_at=now()
WHERE id=$1 AND fence=$2 AND owner_gateway=$3 AND status='connecting' AND expires_at>now() RETURNING *;

-- name: ClaimKubernetesStreamExecution :execrows
UPDATE kubernetes_stream_sessions SET execution_gateway=$3,updated_at=now()
WHERE id=$1 AND fence=$2 AND status='connecting' AND execution_gateway='' AND expires_at>now();

-- name: EndKubernetesStreamSession :one
UPDATE kubernetes_stream_sessions SET status=$3,end_reason=$4,ticket_hash=NULL,ended_at=now(),bytes_in=$5,bytes_out=$6,version=version+1,updated_at=now()
WHERE id=$1 AND fence=$2 AND status IN ('pending','connecting','active','closing') RETURNING *;

-- name: HeartbeatKubernetesStreamSession :execrows
UPDATE kubernetes_stream_sessions SET updated_at=now() WHERE id=$1 AND fence=$2 AND owner_gateway=$3 AND status='active' AND expires_at>now();

-- name: RetireKubernetesStreamSessions :exec
UPDATE kubernetes_stream_sessions SET status='connection_lost',ticket_hash=NULL,ended_at=now(),end_reason='expired_or_owner_lost',version=version+1,updated_at=now()
WHERE status IN ('pending','connecting','active','closing') AND (expires_at<=now() OR status IN ('connecting','active','closing') AND updated_at<now()-interval '45 seconds');

-- name: DeleteRetiredKubernetesStreamSessions :exec
DELETE FROM kubernetes_stream_sessions WHERE expires_at<now()-interval '24 hours' AND status IN ('closed','invalidated','connection_lost');

-- name: RequestCloseKubernetesStreamSession :one
UPDATE kubernetes_stream_sessions SET status='closing',ticket_hash=NULL,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND status IN ('connecting','active') RETURNING *;
