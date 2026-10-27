-- +goose Up
CREATE TABLE kubernetes_namespace_object_identities (
 enterprise_id uuid NOT NULL,cluster_id uuid NOT NULL,object_uid text NOT NULL,
 object_type text NOT NULL,namespace_name text NOT NULL,namespace_uid text NOT NULL,
 observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(cluster_id,object_uid),
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 CHECK(length(object_uid)<=128 AND length(namespace_uid)<=128)
);
CREATE INDEX namespace_object_identity_lookup ON kubernetes_namespace_object_identities(enterprise_id,cluster_id,namespace_name,object_uid);
-- +goose StatementBegin
DO $$ DECLARE role_name text;BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_namespace_object_identities TO %I',role_name);
 END IF;END LOOP;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT SELECT ON kubernetes_namespace_object_identities TO argus_telemetry_ingest;
 END IF;
END $$;
-- +goose StatementEnd
-- +goose Down
DROP TABLE kubernetes_namespace_object_identities;
