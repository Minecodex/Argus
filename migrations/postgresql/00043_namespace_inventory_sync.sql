-- +goose Up
CREATE TABLE kubernetes_namespace_sync_states (
 cluster_id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,
 lease_token uuid,lease_expires_at timestamptz,
 last_attempt_at timestamptz NOT NULL DEFAULT '-infinity',last_success_at timestamptz,
 snapshot_started_at timestamptz,snapshot_resource_version text NOT NULL DEFAULT '',
 observed_cluster_version bigint NOT NULL DEFAULT 0,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','syncing','healthy','error')),
 error_code text NOT NULL DEFAULT '',namespace_count integer NOT NULL DEFAULT 0,
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id)
);
-- +goose StatementBegin
DO $$ DECLARE role_name text;BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_namespace_sync_states TO %I',role_name);
 END IF;END LOOP;END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE kubernetes_namespace_sync_states;
