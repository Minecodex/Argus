-- +goose Up
ALTER TABLE host_runtime_observations ADD COLUMN inventory_json jsonb;
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK (command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe'));

-- +goose Down
ALTER TABLE host_runtime_observations DROP COLUMN inventory_json;
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK (command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure'));
