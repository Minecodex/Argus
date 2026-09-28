-- name: EnsureDashboardRunBudget :exec
INSERT INTO dashboard_run_budgets(run_id,enterprise_id,owner_user_id,scan_remaining,bytes_remaining,rows_remaining,samples_remaining,calls_remaining)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(run_id) DO NOTHING;

-- name: GetDashboardRunBudget :one
SELECT * FROM dashboard_run_budgets WHERE run_id=$1 AND enterprise_id=$2 AND owner_user_id=$3;

-- name: LockDashboardRunBudget :one
SELECT * FROM dashboard_run_budgets WHERE run_id=$1 AND enterprise_id=$2 AND owner_user_id=$3 FOR UPDATE;

-- name: SetDashboardRunBudget :exec
UPDATE dashboard_run_budgets SET scan_remaining=$4,bytes_remaining=$5,rows_remaining=$6,samples_remaining=$7,calls_remaining=$8
WHERE run_id=$1 AND enterprise_id=$2 AND owner_user_id=$3;

-- name: CreateDashboardBudgetReservation :one
INSERT INTO dashboard_budget_reservations(id,run_id,enterprise_id,allocation,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: LockDashboardBudgetReservation :one
SELECT * FROM dashboard_budget_reservations WHERE id=$1 AND run_id=$2 AND enterprise_id=$3 FOR UPDATE;

-- name: SetDashboardBudgetReservation :exec
UPDATE dashboard_budget_reservations SET status=$4,observed=$5 WHERE id=$1 AND run_id=$2 AND enterprise_id=$3 AND status='pending';

-- name: ExpireDashboardBudgetReservations :exec
UPDATE dashboard_budget_reservations SET status='charged_unknown' WHERE run_id=$1 AND enterprise_id=$2 AND status='pending' AND expires_at<=now();

-- name: HasPendingDashboardBudgetReservations :one
SELECT EXISTS(SELECT 1 FROM dashboard_budget_reservations WHERE run_id=$1 AND enterprise_id=$2 AND status='pending');
