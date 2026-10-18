-- +goose Up
ALTER TABLE conversations ADD COLUMN mode text NOT NULL DEFAULT 'investigate' CHECK (mode IN ('investigate','plan'));
ALTER TABLE runs ADD COLUMN mode_snapshot text NOT NULL DEFAULT 'investigate' CHECK (mode_snapshot IN ('investigate','plan'));
ALTER TABLE connector_commands ADD COLUMN dispatch_rejected boolean NOT NULL DEFAULT false;
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe','remote_execution_batch','remote_execution_control','remote_execution_file'));
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution'));

-- The access identity is explicit. A Connector's service account is never an
-- implicit substitute for the account selected when the resource was enrolled.
CREATE TABLE remote_execution_identities (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    resource_type text NOT NULL CHECK (resource_type IN ('host','kubernetes_cluster')),
    resource_id uuid NOT NULL,
    username text NOT NULL,
    identity_sid text NOT NULL DEFAULT '',
    platform text NOT NULL CHECK (platform IN ('linux','windows','kubernetes')),
    credential_id uuid REFERENCES credentials(id),
    credential_version bigint NOT NULL DEFAULT 0,
    connector_id uuid REFERENCES connectors(id),
    binding_source text NOT NULL CHECK (binding_source IN ('ssh_onboarding','kubeconfig','in_cluster','verified_local_account')),
    resource_version bigint NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (enterprise_id, resource_type, resource_id)
);

CREATE TABLE remote_execution_sessions (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    conversation_id uuid NOT NULL REFERENCES conversations(id),
    owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
    resource_type text NOT NULL CHECK (resource_type IN ('host','kubernetes_cluster')),
    resource_id uuid NOT NULL,
    identity_id uuid NOT NULL REFERENCES remote_execution_identities(id),
    identity_version bigint NOT NULL,
    interpreter text NOT NULL CHECK (interpreter IN ('bash','powershell','kubectl','pod_bash')),
    target jsonb NOT NULL DEFAULT '{}',
    target_hash text NOT NULL,
    generation uuid NOT NULL,
    context_revision bigint NOT NULL DEFAULT 0,
    completed_sequence bigint NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'new' CHECK (status IN ('new','active','lost','reclaimed','untrusted')),
    known_directory text NOT NULL DEFAULT '',
    runtime_account text NOT NULL DEFAULT '',
    interpreter_version text NOT NULL DEFAULT '',
    active_request_id uuid,
    last_active_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (enterprise_id,conversation_id,owner_user_id,resource_type,resource_id,identity_id,identity_version,interpreter,target_hash)
);

CREATE TABLE remote_execution_generations (
    session_id uuid NOT NULL REFERENCES remote_execution_sessions(id),
    generation uuid NOT NULL,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    status text NOT NULL DEFAULT 'new' CHECK(status IN ('new','active','lost','untrusted','reclaimed')),
    last_active_at timestamptz NOT NULL DEFAULT now(),
    direct_owner text NOT NULL DEFAULT '',
    PRIMARY KEY(session_id,generation)
);

CREATE TABLE remote_execution_direct_workers (
    id text PRIMARY KEY,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE remote_execution_direct_commands (
    id uuid PRIMARY KEY,
    command_id text NOT NULL UNIQUE,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    identity_id uuid NOT NULL REFERENCES remote_execution_identities(id),
    session_id uuid NOT NULL REFERENCES remote_execution_sessions(id),
    generation uuid NOT NULL,
    command_type text NOT NULL CHECK(command_type IN ('remote_execution_batch','remote_execution_control','remote_execution_file')),
    payload jsonb NOT NULL,
    payload_hash bytea NOT NULL CHECK(octet_length(payload_hash)=32),
    status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','succeeded','failed','result_unknown')),
    owner_instance text NOT NULL DEFAULT '',
    result jsonb NOT NULL DEFAULT '{}',
    error_code text NOT NULL DEFAULT '',
    dispatch_rejected boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(session_id,generation) REFERENCES remote_execution_generations(session_id,generation)
);
CREATE INDEX remote_execution_direct_queue ON remote_execution_direct_commands(status,created_at);

CREATE TABLE remote_execution_requests (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    conversation_id uuid NOT NULL REFERENCES conversations(id),
    owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
    run_id uuid NOT NULL REFERENCES runs(id),
    tool_call_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('connect','plan')),
    revision integer NOT NULL DEFAULT 1,
    supersedes_id uuid REFERENCES remote_execution_requests(id),
    recovery_of_id uuid REFERENCES remote_execution_requests(id),
    action_ref text NOT NULL UNIQUE,
    content_hash text NOT NULL,
    spec jsonb NOT NULL,
    status text NOT NULL DEFAULT 'awaiting_confirmation' CHECK (status IN ('awaiting_confirmation','ready','running','succeeded','failed','partial_failed','result_unknown','paused_for_recovery','stopping','cancelled','superseded','expired')),
    stop_requested boolean NOT NULL DEFAULT false,
    stop_status text NOT NULL DEFAULT '' CHECK(stop_status IN ('','requested','confirmed','unknown')),
    current_step integer NOT NULL DEFAULT 0,
    authorization_version bigint NOT NULL,
    error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (enterprise_id,tool_call_id)
);
ALTER TABLE remote_execution_sessions ADD CONSTRAINT remote_execution_active_request_fk FOREIGN KEY (active_request_id) REFERENCES remote_execution_requests(id);
CREATE UNIQUE INDEX remote_execution_revision_parent ON remote_execution_requests(supersedes_id) WHERE supersedes_id IS NOT NULL;

CREATE TABLE remote_execution_steps (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    request_id uuid NOT NULL REFERENCES remote_execution_requests(id),
    session_id uuid NOT NULL REFERENCES remote_execution_sessions(id),
    generation uuid NOT NULL,
    sequence integer NOT NULL CHECK (sequence > 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','preparing','running','collecting','succeeded','failed','result_unknown','cancelled','skipped','paused_for_recovery')),
    command_id text NOT NULL DEFAULT '',
    result jsonb NOT NULL DEFAULT '{}',
    started_at timestamptz,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (request_id,sequence)
);
CREATE INDEX remote_execution_resource_activity ON remote_execution_sessions (enterprise_id,resource_type,resource_id);
CREATE INDEX remote_execution_step_activity ON remote_execution_steps (session_id,status,updated_at);

CREATE TABLE remote_execution_files (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    request_id uuid NOT NULL REFERENCES remote_execution_requests(id),
    session_id uuid REFERENCES remote_execution_sessions(id),
    direction text NOT NULL CHECK (direction IN ('input','output')),
    path text NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size >= 0),
    content_hash text NOT NULL,
    object_key text NOT NULL,
    status text NOT NULL CHECK (status IN ('sealed','transferring','complete','partial','failed')),
    transferred_bytes bigint NOT NULL DEFAULT 0,
    source_ref text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (request_id,direction,session_id,path)
);
ALTER TABLE executions ADD COLUMN remote_execution_id uuid REFERENCES remote_execution_requests(id);

-- Private model-context evidence. Never included in conversation events or
-- public DTOs: retains only the sanitized concurrent activity actually sent.
CREATE TABLE remote_execution_context_observations (
    model_call_id uuid PRIMARY KEY REFERENCES model_calls(id),
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    run_id uuid NOT NULL REFERENCES runs(id),
    authorization_version bigint NOT NULL,
    projection_hash bytea NOT NULL,
    activity jsonb NOT NULL CHECK (octet_length(activity::text) <= 65536),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX remote_execution_context_run ON remote_execution_context_observations(enterprise_id,run_id,created_at);

INSERT INTO permissions(id,description,registry_version) VALUES
('remote_execution.read','Read authorized remote execution state',13),
('remote_execution.execute','Propose and execute explicitly approved remote batches and plans',13),
('remote_execution.stop','Stop own remote execution',13)
ON CONFLICT(id) DO UPDATE SET description=excluded.description,registry_version=excluded.registry_version;
INSERT INTO role_permissions(role_id,permission_id)
SELECT roles.id,permission.id FROM roles CROSS JOIN (VALUES ('remote_execution.read'),('remote_execution.execute'),('remote_execution.stop')) AS permission(id)
WHERE roles.builtin AND roles.identity_key='enterprise_admin'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM runtime_tasks WHERE queue='remote_execution';
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification'));
DELETE FROM connector_commands WHERE command_type IN ('remote_execution_batch','remote_execution_control','remote_execution_file');
ALTER TABLE connector_commands DROP CONSTRAINT connector_commands_command_type_check;
ALTER TABLE connector_commands ADD CONSTRAINT connector_commands_command_type_check CHECK(command_type IN ('host_connection_probe','kubernetes_connection_probe','kubernetes_resource_query','kubernetes_pod_logs','connector_uninstall','collector_management','host_connector_install','host_connector_removal','host_windows_rdp_configure','host_inventory_probe'));
DELETE FROM role_permissions WHERE permission_id IN ('remote_execution.read','remote_execution.execute','remote_execution.stop');
DELETE FROM permissions WHERE id IN ('remote_execution.read','remote_execution.execute','remote_execution.stop');
ALTER TABLE executions DROP COLUMN remote_execution_id;
ALTER TABLE remote_execution_sessions DROP CONSTRAINT remote_execution_active_request_fk;
DROP TABLE remote_execution_context_observations;
DROP TABLE remote_execution_files,remote_execution_steps,remote_execution_requests,remote_execution_direct_commands,remote_execution_generations,remote_execution_direct_workers,remote_execution_sessions,remote_execution_identities;
ALTER TABLE runs DROP COLUMN mode_snapshot;
ALTER TABLE connector_commands DROP COLUMN dispatch_rejected;
ALTER TABLE conversations DROP COLUMN mode;
