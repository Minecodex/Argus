-- +goose Up
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_sources_resource_type_check;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_sources_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster','monitoring_target','application'));
ALTER TABLE telemetry_sources ADD COLUMN collection_id uuid;
ALTER TABLE telemetry_sources ADD COLUMN collection_revision bigint;
ALTER TABLE telemetry_sources ADD COLUMN assignment_epoch bigint;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_source_collection FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id);
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_source_collection_revision FOREIGN KEY(collection_id,collection_revision) REFERENCES monitoring_collection_revisions(collection_id,revision);
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_source_assignment CHECK(
 (collection_id IS NULL AND collection_revision IS NULL AND assignment_epoch IS NULL AND resource_type IN ('host','kubernetes_cluster'))
 OR (collection_id IS NOT NULL AND collection_revision>0 AND assignment_epoch>0)
);
CREATE UNIQUE INDEX monitoring_target_active_collection ON monitoring_collections(enterprise_id,target_type,target_id,integration_key)
 WHERE status='active' AND target_type='monitoring_target';

ALTER TABLE credential_leases DROP CONSTRAINT credential_leases_protocol_check;
ALTER TABLE credential_leases ADD CONSTRAINT credential_leases_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification','monitoring'));
CREATE TABLE monitoring_credential_deliveries (
 lease_id uuid PRIMARY KEY REFERENCES credential_leases(id), enterprise_id uuid NOT NULL,
 collection_id uuid NOT NULL, collection_revision bigint NOT NULL, assignment_epoch bigint NOT NULL,
 collector_id uuid NOT NULL, collector_revision bigint NOT NULL,
 FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id),
 FOREIGN KEY(collection_id,collection_revision) REFERENCES monitoring_collection_revisions(collection_id,revision),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 FOREIGN KEY(collector_id,collector_revision) REFERENCES collector_config_revisions(collector_id,revision)
);
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT ON monitoring_credential_deliveries TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM telemetry_sources WHERE collection_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM monitoring_credential_deliveries)
 OR EXISTS(SELECT 1 FROM credential_leases WHERE protocol='monitoring') THEN
  RAISE EXCEPTION 'monitoring runtime data must be explicitly retired before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE monitoring_credential_deliveries;
ALTER TABLE credential_leases DROP CONSTRAINT credential_leases_protocol_check;
ALTER TABLE credential_leases ADD CONSTRAINT credential_leases_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification'));
DROP INDEX monitoring_target_active_collection;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_source_assignment;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_source_collection_revision;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_source_collection;
ALTER TABLE telemetry_sources DROP COLUMN assignment_epoch;
ALTER TABLE telemetry_sources DROP COLUMN collection_revision;
ALTER TABLE telemetry_sources DROP COLUMN collection_id;
ALTER TABLE telemetry_sources DROP CONSTRAINT telemetry_sources_resource_type_check;
ALTER TABLE telemetry_sources ADD CONSTRAINT telemetry_sources_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster'));
