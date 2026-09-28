-- +goose Up
ALTER TABLE collector_instances ADD COLUMN source_generation uuid NOT NULL DEFAULT gen_random_uuid();
CREATE TABLE telemetry_sources (
 id uuid NOT NULL, enterprise_id uuid NOT NULL REFERENCES enterprises(id), collector_id uuid NOT NULL REFERENCES collector_instances(id),
 generation uuid NOT NULL, resource_type text NOT NULL CHECK(resource_type IN ('host','kubernetes_cluster')), resource_id uuid NOT NULL,
 source_key text NOT NULL, source_type text NOT NULL, signals text[] NOT NULL,
 config_revision bigint NOT NULL CHECK(config_revision>0), config_hash bytea NOT NULL CHECK(octet_length(config_hash)=32),
 capability_version text NOT NULL DEFAULT 'v1', created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(id,config_revision), UNIQUE(collector_id,generation,source_key,config_revision),
 FOREIGN KEY(collector_id,config_revision) REFERENCES collector_config_revisions(collector_id,revision)
);
CREATE INDEX telemetry_sources_scope_idx ON telemetry_sources(enterprise_id,resource_id,source_type);
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT ON telemetry_sources TO %I',role_name);
  END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT SELECT ON telemetry_sources TO argus_telemetry_ingest;
 END IF;
END $roles$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE telemetry_sources;
ALTER TABLE collector_instances DROP COLUMN source_generation;
