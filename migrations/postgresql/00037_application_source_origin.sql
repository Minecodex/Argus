-- +goose Up
-- A source has exactly one origin: infrastructure, an assigned collection, or
-- an application endpoint. Endpoint sources do not invent collection epochs.
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_source_assignment;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_source_assignment CHECK(
 (collection_id IS NULL AND collection_revision IS NULL AND assignment_epoch IS NULL
  AND ingest_endpoint_id IS NULL AND ingest_endpoint_version IS NULL
  AND resource_type IN ('host','kubernetes_cluster'))
 OR (collection_id IS NOT NULL AND collection_revision IS NOT NULL AND collection_revision>0
  AND assignment_epoch IS NOT NULL AND assignment_epoch>0
  AND ingest_endpoint_id IS NULL AND ingest_endpoint_version IS NULL)
 OR (ingest_endpoint_id IS NOT NULL AND ingest_endpoint_version IS NOT NULL AND ingest_endpoint_version>0
  AND collection_id IS NULL AND collection_revision IS NULL AND assignment_epoch IS NULL
  AND application_service_id IS NULL AND resource_type='application')
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM telemetry_sources WHERE ingest_endpoint_id IS NOT NULL) THEN
  RAISE EXCEPTION 'application endpoint sources exist; do not remove their origin contract';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_source_assignment;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_source_assignment CHECK(
 (collection_id IS NULL AND collection_revision IS NULL AND assignment_epoch IS NULL AND resource_type IN ('host','kubernetes_cluster'))
 OR (collection_id IS NOT NULL AND collection_revision>0 AND assignment_epoch>0)
);
