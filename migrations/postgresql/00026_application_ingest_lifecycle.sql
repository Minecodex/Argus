-- +goose Up
ALTER TABLE monitoring_ingest_keys ADD COLUMN revealed_at timestamptz;
ALTER TABLE monitoring_ingest_endpoints ADD COLUMN last_received_at timestamptz;
ALTER TABLE monitoring_ingest_endpoints ADD COLUMN last_rejected_at timestamptz;
ALTER TABLE monitoring_ingest_endpoints ADD COLUMN last_error_code text NOT NULL DEFAULT '';

-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT UPDATE(last_received_at,last_rejected_at,last_error_code) ON monitoring_ingest_endpoints TO argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE monitoring_ingest_endpoints DROP COLUMN last_received_at,DROP COLUMN last_rejected_at,DROP COLUMN last_error_code;
ALTER TABLE monitoring_ingest_keys DROP COLUMN revealed_at;
