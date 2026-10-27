-- +goose Up
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_resource_mutation','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe','remote_execution_batch','remote_execution_control','remote_execution_file'));
CREATE TABLE kubernetes_operations (
 id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,cluster_id uuid NOT NULL,
 namespace_id uuid,namespace_uid text NOT NULL DEFAULT '',
 actor_id uuid NOT NULL,actor_type text NOT NULL CHECK(actor_type IN ('user','service_account')),
 authorization_version bigint NOT NULL,cluster_version bigint NOT NULL,
 pending_action_id uuid,status text NOT NULL CHECK(status IN ('awaiting_confirmation','queued','running','observing','succeeded','failed','cancelled','result_unknown')),
 operation text NOT NULL,type_key text NOT NULL,target_name text NOT NULL,target_uid text NOT NULL DEFAULT '',expected_resource_version text NOT NULL DEFAULT '',
 private_plan jsonb NOT NULL CHECK(jsonb_typeof(private_plan)='object'),result jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(result)='object'),
 error_code text NOT NULL DEFAULT '',connector_command_id uuid,
 version bigint NOT NULL DEFAULT 1,started_at timestamptz,finished_at timestamptz,
 heartbeat_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 FOREIGN KEY(namespace_id,enterprise_id) REFERENCES kubernetes_namespace_resources(id,enterprise_id),
 FOREIGN KEY(pending_action_id) REFERENCES pending_actions(id),
 FOREIGN KEY(connector_command_id) REFERENCES connector_commands(id),UNIQUE(id,enterprise_id)
);
CREATE INDEX kubernetes_operations_reconcile ON kubernetes_operations(status,heartbeat_at,created_at);
ALTER TABLE executions ADD COLUMN kubernetes_operation_id uuid REFERENCES kubernetes_operations(id);
-- +goose StatementBegin
DO $$ DECLARE role_name text; BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON kubernetes_operations TO %I',role_name);
  END IF;
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM kubernetes_operations) THEN RAISE EXCEPTION 'Kubernetes operation history exists; export or explicitly remove owned test records before downgrade';END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE executions DROP COLUMN kubernetes_operation_id;
DROP TABLE kubernetes_operations;
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe','remote_execution_batch','remote_execution_control','remote_execution_file'));
