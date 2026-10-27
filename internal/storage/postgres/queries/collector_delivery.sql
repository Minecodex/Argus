-- name: LockCollectorDeliveryOwner :one
WITH admission_lock AS MATERIALIZED (SELECT pg_advisory_xact_lock(hashtextextended((sqlc.arg(id)::uuid)::text,0)))
SELECT c.id FROM collector_instances c,admission_lock WHERE c.enterprise_id=sqlc.arg(enterprise_id) AND c.id=sqlc.arg(id)::uuid;

-- name: EnsureCollectorDeliveryReport :one
INSERT INTO collector_delivery_reports(id,enterprise_id,collector_id,instance_id)
SELECT $1,$2,$3,$4 WHERE EXISTS(SELECT 1 FROM collector_delivery_reports WHERE collector_id=$3 AND instance_id=$4)
 OR (SELECT count(*) FROM collector_delivery_reports WHERE collector_id=$3)<4096
ON CONFLICT(collector_id,instance_id) DO UPDATE SET instance_id=EXCLUDED.instance_id RETURNING *;

-- name: UpdateCollectorDeliveryReport :exec
UPDATE collector_delivery_reports SET sequence=$3,report=$4,pending_alert=pending_alert OR sqlc.arg(pending_alert)::boolean,received_at=now() WHERE enterprise_id=$1 AND id=$2;

-- name: LockCollectorDeliveryReport :one
SELECT * FROM collector_delivery_reports WHERE enterprise_id=$1 AND id=$2 FOR UPDATE;

-- name: MarkCollectorDeliveryNotified :exec
UPDATE collector_delivery_reports SET notified_sequence=sequence,notified_report=report,pending_alert=false WHERE enterprise_id=$1 AND id=$2;

-- name: ListCollectorDeliveryReports :many
SELECT * FROM collector_delivery_reports WHERE enterprise_id=$1 AND collector_id=$2 ORDER BY received_at DESC,id LIMIT 4096;

-- name: DueCollectorDeliveryAlerts :many
SELECT r.* FROM collector_delivery_reports r WHERE pending_alert AND NOT EXISTS(
 SELECT 1 FROM runtime_tasks t WHERE t.enterprise_id=r.enterprise_id AND t.queue='alert.collector_delivery'
 AND t.payload->>'report_id'=r.id::text
 AND (t.status IN ('pending','leased','running') OR (t.status='failed' AND t.updated_at>now()-interval '5 minutes'))
) ORDER BY r.received_at,r.id LIMIT 100 FOR UPDATE OF r SKIP LOCKED;

-- name: BindCollectorDeliveryPolicy :exec
INSERT INTO collector_delivery_policies(policy_id,collector_id,enterprise_id) VALUES($1,$2,$3)
ON CONFLICT(collector_id) DO NOTHING;

-- name: IsCollectorDeliveryPolicy :one
SELECT EXISTS(SELECT 1 FROM collector_delivery_policies WHERE enterprise_id=$1 AND policy_id=$2);

-- name: EnsureCollectorDeliveryPolicy :one
INSERT INTO alert_configurations(id,enterprise_id,kind,name,spec,created_by)
VALUES($1,$2,'policy','Collector delivery data gap',$3,'00000000-0000-0000-0000-000000000000')
ON CONFLICT(id) DO UPDATE SET id=EXCLUDED.id RETURNING *;
