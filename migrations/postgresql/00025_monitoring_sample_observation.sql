-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT UPDATE(last_sample_at,last_attempt_at,observed_at,health) ON monitoring_collections TO argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  REVOKE UPDATE(last_sample_at,last_attempt_at,observed_at,health) ON monitoring_collections FROM argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd
