-- +goose Up
-- Ingest verifies native Namespace UIDs against authoritative inventory. It
-- needs metadata reads only; synchronization writes remain with the worker.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT SELECT ON kubernetes_namespace_resources TO argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  REVOKE SELECT ON kubernetes_namespace_resources FROM argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd
