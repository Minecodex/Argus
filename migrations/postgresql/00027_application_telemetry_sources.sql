-- +goose Up
ALTER TABLE telemetry_sources ADD COLUMN ingest_endpoint_id uuid;
ALTER TABLE telemetry_sources ADD COLUMN ingest_endpoint_version bigint;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_application_endpoint_fk
 FOREIGN KEY(ingest_endpoint_id,enterprise_id) REFERENCES monitoring_ingest_endpoints(id,enterprise_id);
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_application_source_binding CHECK(
 (ingest_endpoint_id IS NULL AND ingest_endpoint_version IS NULL)
 OR (ingest_endpoint_id IS NOT NULL AND ingest_endpoint_version>0 AND collection_id IS NULL AND resource_type='application'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM telemetry_sources WHERE ingest_endpoint_id IS NOT NULL) THEN
  RAISE EXCEPTION 'application telemetry sources exist; export or explicitly remove owned test data before downgrade';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_application_source_binding,DROP CONSTRAINT telemetry_application_endpoint_fk;
ALTER TABLE telemetry_sources DROP COLUMN ingest_endpoint_id,DROP COLUMN ingest_endpoint_version;
