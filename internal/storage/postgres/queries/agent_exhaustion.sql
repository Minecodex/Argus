-- name: FailActiveExhaustedRunStep :exec
UPDATE run_steps SET status='failed',version=version+1,updated_at=now()
WHERE id=sqlc.arg(id) AND enterprise_id=sqlc.arg(enterprise_id) AND run_id=sqlc.arg(run_id)
 AND status IN ('pending','leased','running','waiting_input','waiting_approval');
