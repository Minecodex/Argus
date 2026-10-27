-- +goose Up
ALTER TABLE telemetry_sources ADD COLUMN application_service_id uuid;
ALTER TABLE telemetry_sources ADD COLUMN namespace_uid text NOT NULL DEFAULT '';
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_log_service_scope CHECK(application_service_id IS NULL OR (resource_type='application' AND signals=ARRAY['logs']::text[]));
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_log_service_owner FOREIGN KEY(application_service_id,resource_id,enterprise_id) REFERENCES monitoring_application_services(id,application_id,enterprise_id);
CREATE INDEX telemetry_log_namespace_scope ON telemetry_sources(enterprise_id,namespace_uid,collector_id) WHERE namespace_uid<>'';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM telemetry_sources WHERE application_service_id IS NOT NULL OR namespace_uid<>'') THEN
  RAISE EXCEPTION 'Log source ownership exists; do not discard historical application or namespace identity';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX telemetry_log_namespace_scope;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_log_service_owner;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_log_service_scope;
ALTER TABLE telemetry_sources DROP COLUMN application_service_id;
ALTER TABLE telemetry_sources DROP COLUMN namespace_uid;
