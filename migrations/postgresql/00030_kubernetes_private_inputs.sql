-- +goose Up
CREATE TABLE kubernetes_private_inputs (
 id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,operation_id uuid NOT NULL,cluster_id uuid NOT NULL,
 namespace_id uuid NOT NULL,namespace_uid text NOT NULL,
 actor_id uuid NOT NULL,actor_type text NOT NULL CHECK(actor_type IN ('user','service_account')),
 authorization_version bigint NOT NULL,mutation_hash bytea NOT NULL CHECK(octet_length(mutation_hash)=32),
 envelope jsonb NOT NULL CHECK(jsonb_typeof(envelope)='object'),
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','bound')),
 expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 FOREIGN KEY(namespace_id,enterprise_id) REFERENCES kubernetes_namespace_resources(id,enterprise_id),
 UNIQUE(id,enterprise_id)
);
CREATE INDEX kubernetes_private_inputs_expiry ON kubernetes_private_inputs(expires_at);
-- +goose StatementBegin
DO $$ DECLARE role_name text; BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_private_inputs TO %I',role_name);
  END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM kubernetes_private_inputs) THEN RAISE EXCEPTION 'Retire owned Kubernetes private inputs before downgrade';END IF;
END $$;
-- +goose StatementEnd
DROP TABLE kubernetes_private_inputs;
