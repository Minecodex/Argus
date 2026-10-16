-- +goose Up
ALTER TABLE alert_instances ADD COLUMN handling_version bigint NOT NULL DEFAULT 1;
ALTER TABLE alert_evaluations ADD COLUMN result jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE alert_notification_groups (
 enterprise_id uuid NOT NULL REFERENCES enterprises(id), channel_id uuid NOT NULL REFERENCES alert_configurations(id),
 group_key text NOT NULL, next_send_at timestamptz NOT NULL, last_sent_at timestamptz,
 PRIMARY KEY(enterprise_id,channel_id,group_key)
);
CREATE TABLE alert_delivery_batches (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id), channel_id uuid NOT NULL REFERENCES alert_configurations(id),
 channel_version bigint NOT NULL, group_key text NOT NULL, owner_task_id uuid NOT NULL REFERENCES runtime_tasks(id), fence bigint NOT NULL,
 payload jsonb NOT NULL, status text NOT NULL CHECK(status IN ('prepared','sending','accepted','failed','unknown','canceled')),
 send_started_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
ALTER TABLE alert_deliveries ADD COLUMN batch_id uuid REFERENCES alert_delivery_batches(id);
ALTER TABLE alert_deliveries ADD COLUMN retry_until timestamptz NOT NULL DEFAULT (now()+interval '24 hours');
ALTER TABLE alert_deliveries ADD COLUMN resend_of_id uuid REFERENCES alert_deliveries(id);
CREATE INDEX alert_deliveries_batch ON alert_deliveries(enterprise_id,batch_id);
CREATE INDEX alert_instances_history ON alert_instances(enterprise_id,created_at DESC,id DESC);
CREATE INDEX alert_deliveries_history ON alert_deliveries(enterprise_id,created_at DESC,id DESC);
CREATE INDEX alert_batches_recovery ON alert_delivery_batches(owner_task_id,status) WHERE status IN ('prepared','sending');

-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON alert_delivery_batches,alert_notification_groups TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX alert_instances_history;
DROP INDEX alert_deliveries_history;
ALTER TABLE alert_deliveries DROP COLUMN resend_of_id,DROP COLUMN retry_until,DROP COLUMN batch_id;
DROP TABLE alert_delivery_batches;
DROP TABLE alert_notification_groups;
ALTER TABLE alert_evaluations DROP COLUMN result;
ALTER TABLE alert_instances DROP COLUMN handling_version;
