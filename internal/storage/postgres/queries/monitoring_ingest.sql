-- name: CreateMonitoringIngestEndpoint :one
INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,collector_id,service_allowlist,created_by,enabled)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetMonitoringIngestEndpoint :one
SELECT * FROM monitoring_ingest_endpoints WHERE enterprise_id=$1 AND id=$2;

-- name: LockMonitoringIngestEndpoint :one
SELECT * FROM monitoring_ingest_endpoints WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: ListCollectorApplicationEndpoints :many
SELECT * FROM monitoring_ingest_endpoints WHERE enterprise_id=$1 AND collector_id=$2 AND enabled ORDER BY id;

-- name: ListMonitoringIngestEndpoints :many
SELECT * FROM monitoring_ingest_endpoints WHERE enterprise_id=$1 AND application_id=$2 AND id>sqlc.arg(after_id)::uuid ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: UpdateMonitoringIngestEndpoint :one
UPDATE monitoring_ingest_endpoints SET enabled=$4,name=$5,service_allowlist=$6,version=version+1,updated_at=now()
WHERE enterprise_id=$1 AND id=$2 AND version=$3 RETURNING *;

-- name: CreateMonitoringIngestKey :one
INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: ListMonitoringIngestKeys :many
SELECT id,enterprise_id,endpoint_id,application_id,key_prefix,expires_at,revoked_at,last_used_at,version,created_by,created_at,revealed_at
FROM monitoring_ingest_keys WHERE enterprise_id=$1 AND endpoint_id=$2 AND id>sqlc.arg(after_id)::uuid ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: GetMonitoringIngestKey :one
SELECT * FROM monitoring_ingest_keys WHERE enterprise_id=$1 AND id=$2;

-- name: ConsumeMonitoringKeyReveal :one
UPDATE monitoring_ingest_keys SET revealed_at=now()
WHERE enterprise_id=$1 AND id=$2 AND created_by=$3 AND revealed_at IS NULL AND revoked_at IS NULL
 AND created_at>now()-interval '10 minutes' AND (expires_at IS NULL OR expires_at>now())
RETURNING *;

-- name: CountMonitoringEndpointKeys :one
SELECT count(*)::bigint FROM monitoring_ingest_keys WHERE enterprise_id=$1 AND endpoint_id=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now());

-- name: ObserveApplicationIngest :exec
UPDATE monitoring_ingest_endpoints SET last_received_at=now(),last_error_code=''
WHERE enterprise_id=$1 AND id=$2 AND enabled;

-- name: RejectApplicationIngest :exec
UPDATE monitoring_ingest_endpoints SET last_rejected_at=now(),last_error_code=$3
WHERE enterprise_id=$1 AND id=$2;

-- name: ResolveMonitoringIngestKey :one
SELECT k.*,e.protocol,e.signals,e.collector_id,e.service_allowlist,e.version AS endpoint_version,a.environment,
 d.version AS distribution_version,d.collector_version,d.config_schema_version,d.support_status,d.components
FROM monitoring_ingest_keys k
JOIN monitoring_ingest_endpoints e ON e.id=k.endpoint_id AND e.enterprise_id=k.enterprise_id AND e.application_id=k.application_id
JOIN monitoring_applications a ON a.id=k.application_id AND a.enterprise_id=k.enterprise_id
JOIN collector_instances c ON c.id=e.collector_id AND c.enterprise_id=e.enterprise_id
JOIN collector_distribution_versions d ON d.id=c.distribution_version_id
WHERE k.id=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND e.enabled AND a.status='active'
 AND c.status NOT IN ('uninstalled','uninstalling','result_unknown');

-- name: RevokeMonitoringIngestKey :execrows
UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1
WHERE enterprise_id=$1 AND id=$2 AND version=$3 AND revoked_at IS NULL;

-- name: TouchMonitoringIngestKey :exec
UPDATE monitoring_ingest_keys SET last_used_at=now() WHERE id=$1 AND enterprise_id=$2 AND revoked_at IS NULL;

-- name: DisableApplicationIngest :exec
UPDATE monitoring_ingest_endpoints SET enabled=false,version=version+1,updated_at=now() WHERE enterprise_id=$1 AND application_id=$2;
