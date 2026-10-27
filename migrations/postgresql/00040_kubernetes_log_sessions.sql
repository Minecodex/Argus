-- +goose Up
CREATE TABLE kubernetes_log_sessions (
 id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,cluster_id uuid NOT NULL,
 namespace_id uuid NOT NULL,namespace_uid text NOT NULL,
 actor_id uuid NOT NULL,http_session_id uuid NOT NULL REFERENCES sessions(id),
 authorization_version bigint NOT NULL,cluster_version bigint NOT NULL,
 binding jsonb NOT NULL CHECK(jsonb_typeof(binding)='object'),
 ticket_hash bytea CHECK(ticket_hash IS NULL OR octet_length(ticket_hash)=32),
 ticket_expires_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','active','closed','invalidated')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 FOREIGN KEY(namespace_id,enterprise_id) REFERENCES kubernetes_namespace_resources(id,enterprise_id),
 FOREIGN KEY(actor_id,enterprise_id) REFERENCES enterprise_users(id,enterprise_id),
 UNIQUE(id,enterprise_id)
);
CREATE INDEX kubernetes_log_sessions_actor ON kubernetes_log_sessions(enterprise_id,actor_id,expires_at) WHERE status IN ('pending','active');
-- +goose StatementBegin
DO $$ DECLARE role_name text; BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_log_sessions TO %I',role_name);
  END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM kubernetes_log_sessions) THEN RAISE EXCEPTION 'Retire owned Kubernetes log sessions before downgrade';END IF;
END $$;
-- +goose StatementEnd
DROP TABLE kubernetes_log_sessions;
