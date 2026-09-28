-- +goose Up
CREATE TABLE dashboard_run_budgets (
 run_id uuid PRIMARY KEY REFERENCES runs(id), enterprise_id uuid NOT NULL, owner_user_id uuid NOT NULL,
 scan_remaining bigint NOT NULL CHECK(scan_remaining>=0), bytes_remaining bigint NOT NULL CHECK(bytes_remaining>=0),
 rows_remaining bigint NOT NULL CHECK(rows_remaining>=0), samples_remaining bigint NOT NULL CHECK(samples_remaining>=0),
 calls_remaining bigint NOT NULL CHECK(calls_remaining>=0), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE dashboard_budget_reservations (
 id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES dashboard_run_budgets(run_id), enterprise_id uuid NOT NULL,
 allocation jsonb NOT NULL, observed jsonb, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','settled','charged_unknown')),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dashboard_budget_pending ON dashboard_budget_reservations(run_id,expires_at) WHERE status='pending';
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON dashboard_run_budgets,dashboard_budget_reservations TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd
-- +goose Down
DROP TABLE dashboard_budget_reservations;
DROP TABLE dashboard_run_budgets;
