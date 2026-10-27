-- +goose Up
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_resource_mutation','kubernetes_pod_logs','kubernetes_pinned_pod_logs','kubernetes_helm','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe','remote_execution_batch','remote_execution_control','remote_execution_file'));
CREATE TABLE kubernetes_helm_inputs (
 id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,operation_id uuid NOT NULL,
 cluster_id uuid NOT NULL,namespace_id uuid NOT NULL,namespace_uid text NOT NULL,
 actor_id uuid NOT NULL,actor_type text NOT NULL CHECK(actor_type IN ('user','service_account')),
 authorization_version bigint NOT NULL,request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 envelope jsonb NOT NULL CHECK(jsonb_typeof(envelope)='object'),
 status text NOT NULL DEFAULT 'staged' CHECK(status IN ('staged','bound')),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '1 hour',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 FOREIGN KEY(namespace_id,enterprise_id) REFERENCES kubernetes_namespace_resources(id,enterprise_id),UNIQUE(id,enterprise_id)
);
CREATE INDEX kubernetes_helm_inputs_retire ON kubernetes_helm_inputs(expires_at);
CREATE TABLE kubernetes_helm_locks (
 enterprise_id uuid NOT NULL,cluster_id uuid NOT NULL,namespace_uid text NOT NULL,release_name text NOT NULL,
 operation_id uuid NOT NULL,fence bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(cluster_id,namespace_uid,release_name),
 FOREIGN KEY(operation_id,enterprise_id) REFERENCES kubernetes_operations(id,enterprise_id)
);
-- +goose StatementBegin
DO $$ DECLARE role_name text;BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_helm_inputs,kubernetes_helm_locks TO %I',role_name);
 END IF;END LOOP;END $$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM kubernetes_helm_inputs) OR EXISTS(SELECT 1 FROM kubernetes_operations WHERE type_key='helm_release') THEN RAISE EXCEPTION 'Retire owned Helm operations and private inputs before downgrade';END IF;END $$;
-- +goose StatementEnd
DROP TABLE kubernetes_helm_locks;
DROP TABLE kubernetes_helm_inputs;
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_resource_mutation','kubernetes_pod_logs','kubernetes_pinned_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe','remote_execution_batch','remote_execution_control','remote_execution_file'));
