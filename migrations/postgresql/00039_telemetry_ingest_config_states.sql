-- +goose Up
-- Ingest needs apply state, not rendered configurations or control secrets.
CREATE VIEW telemetry_ingest_config_states WITH (security_barrier=true) AS
 SELECT collector_id,revision,status FROM collector_config_revisions;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT SELECT ON telemetry_ingest_config_states TO argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP VIEW telemetry_ingest_config_states;
