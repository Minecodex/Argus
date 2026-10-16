-- +goose Up
CREATE TABLE alert_configurations (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    kind text NOT NULL CHECK (kind IN ('policy','silence','inhibition','channel','route','settings','template','grouping')),
    name text NOT NULL,
    spec jsonb NOT NULL CHECK (jsonb_typeof(spec) = 'object'),
    version bigint NOT NULL DEFAULT 1,
    generation bigint NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deleted')),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    next_evaluation_at timestamptz NOT NULL DEFAULT now(),
    evaluation_health text NOT NULL DEFAULT 'not_evaluated',
    evaluation_error text NOT NULL DEFAULT '',
    last_evaluated_at timestamptz,
    last_successful_at timestamptz,
    UNIQUE (enterprise_id,kind,id)
);
CREATE INDEX alert_configurations_list ON alert_configurations(enterprise_id,kind,status,updated_at DESC,id);
CREATE INDEX alert_configurations_due ON alert_configurations(next_evaluation_at) WHERE kind='policy' AND status='active';
CREATE TABLE alert_configuration_revisions (
    configuration_id uuid NOT NULL REFERENCES alert_configurations(id),
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    version bigint NOT NULL,
    name text NOT NULL,
    spec jsonb NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(configuration_id,version)
);
CREATE TABLE alert_evaluations (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id), policy_id uuid NOT NULL REFERENCES alert_configurations(id),
 policy_version bigint NOT NULL, slot timestamptz NOT NULL, evaluator_id uuid, authorization_version bigint NOT NULL,
 scope jsonb NOT NULL, health text NOT NULL, error_code text NOT NULL, fence bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(enterprise_id,policy_id,slot)
);
CREATE TABLE alert_instances (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    policy_id uuid NOT NULL REFERENCES alert_configurations(id),
    policy_version bigint NOT NULL,
    generation bigint NOT NULL,
    fingerprint text NOT NULL,
    resource_ids uuid[] NOT NULL,
    source_ids uuid[] NOT NULL DEFAULT '{}',
    state jsonb NOT NULL,
    lifecycle text NOT NULL CHECK(lifecycle IN ('normal','pending','firing','recovering','resolved','closed')),
    health text NOT NULL DEFAULT 'normal',
    severity text NOT NULL CHECK(severity IN ('critical','warning','info')),
    acknowledged boolean NOT NULL DEFAULT false,
    owner_user_id uuid,
    started_at timestamptz,
    resolved_at timestamptz,
    last_evaluated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX alert_instance_active_fingerprint ON alert_instances(enterprise_id,policy_id,fingerprint) WHERE lifecycle NOT IN ('resolved','closed');
CREATE INDEX alert_instances_list ON alert_instances(enterprise_id,lifecycle,updated_at DESC,id);
CREATE INDEX alert_instances_resources ON alert_instances USING gin(resource_ids);
CREATE TABLE alert_events (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    instance_id uuid NOT NULL REFERENCES alert_instances(id),
    policy_id uuid NOT NULL,
    event_type text NOT NULL,
    resource_ids uuid[] NOT NULL,
    payload jsonb NOT NULL,
    actor_id uuid,
    origin text NOT NULL CHECK(origin IN ('evaluator','admin_ui','model_agent')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alert_events_history ON alert_events(enterprise_id,instance_id,created_at,id);
CREATE TABLE alert_channel_rate_limits (enterprise_id uuid NOT NULL REFERENCES enterprises(id),channel_id uuid PRIMARY KEY REFERENCES alert_configurations(id),not_before timestamptz NOT NULL);
CREATE TABLE alert_deliveries (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    event_id uuid REFERENCES alert_events(id),
    instance_id uuid REFERENCES alert_instances(id),
    channel_id uuid NOT NULL REFERENCES alert_configurations(id),
    channel_version bigint NOT NULL,
    route_version bigint NOT NULL,
    dedup_key text NOT NULL,
    group_key text NOT NULL,
    resource_ids uuid[] NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','accepted','failed','unknown','muted','inhibited','canceled')),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempt integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    last_attempt_at timestamptz,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(enterprise_id,dedup_key)
);
CREATE INDEX alert_deliveries_due ON alert_deliveries(available_at,status);
CREATE INDEX alert_deliveries_list ON alert_deliveries(enterprise_id,updated_at DESC,id);
CREATE TABLE alert_delivery_attempts (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    delivery_id uuid NOT NULL REFERENCES alert_deliveries(id),
    attempt integer NOT NULL,
    status text NOT NULL,
    result_code text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL,
    UNIQUE(delivery_id,attempt)
);
INSERT INTO permissions (id,description,registry_version)
VALUES ('alert.read','Read authorized alerts and histories',12),
       ('alert.policy.manage','Manage alert policies and rules',12),
       ('alert.silence.manage','Manage alert silences and inhibitions',12),
       ('alert.notification.manage','Manage alert channels, routes and deliveries',12),
       ('alert.handle','Acknowledge, assign and comment on authorized alerts',12)
ON CONFLICT (id) DO NOTHING;

UPDATE permissions SET registry_version=12;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r JOIN permissions p ON p.id LIKE 'alert.%'
WHERE r.builtin AND (r.identity_key IN ('enterprise_admin','resource_admin')
OR r.identity_key='resource_operator' AND p.id IN ('alert.read','alert.policy.manage','alert.silence.manage','alert.handle')
OR r.identity_key='resource_viewer' AND p.id='alert.read') ON CONFLICT DO NOTHING;
UPDATE roles SET version=version+1,updated_at=now() WHERE builtin AND identity_key IN ('enterprise_admin','resource_admin','resource_operator','resource_viewer');
UPDATE enterprise_users SET authorization_version=authorization_version+1,updated_at=now();
UPDATE service_accounts SET authorization_version=authorization_version+1,updated_at=now();

ALTER TABLE credentials DROP CONSTRAINT credentials_protocol_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification'));
ALTER TABLE credential_leases DROP CONSTRAINT credential_leases_protocol_check;
ALTER TABLE credential_leases ADD CONSTRAINT credential_leases_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification'));
ALTER TABLE credential_leases DROP CONSTRAINT credential_leases_recipient_type_check;
ALTER TABLE credential_leases ADD CONSTRAINT credential_leases_recipient_type_check CHECK(recipient_type IN ('connector','direct_executor','connector_gateway','mcp_adapter','notification_worker'));
ALTER TABLE secrets DROP CONSTRAINT secrets_type_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_type_check CHECK(type IN ('ssh_password','ssh_private_key','windows_password','kubeconfig','api_token','basic_auth','notification'));


ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification'));
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON alert_configurations,alert_instances,alert_deliveries,alert_channel_rate_limits TO %I',role_name);
   EXECUTE format('GRANT SELECT,INSERT ON alert_configuration_revisions,alert_events,alert_delivery_attempts,alert_evaluations TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE alert_delivery_attempts;
DROP TABLE alert_deliveries;
DROP TABLE alert_channel_rate_limits;
DROP TABLE alert_events;
DROP TABLE alert_instances;
DROP TABLE alert_evaluations;
DROP TABLE alert_configuration_revisions;
DROP TABLE alert_configurations;
DELETE FROM role_permissions WHERE permission_id LIKE 'alert.%';
DELETE FROM permissions WHERE id LIKE 'alert.%';

ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
DELETE FROM runtime_tasks WHERE queue IN ('alert-evaluation','alert-notification');
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query'));
-- Notification credentials are retained on rollback. Narrowing their protocol
-- constraints would delete or invalidate unrelated encrypted credential data.
UPDATE permissions SET registry_version=11;
