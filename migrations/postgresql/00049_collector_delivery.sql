-- +goose Up
CREATE TABLE collector_delivery_reports (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, collector_id uuid NOT NULL,
 instance_id uuid NOT NULL, sequence bigint NOT NULL DEFAULT 0 CHECK(sequence>=0),
 report jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(report)='object'),
 notified_sequence bigint NOT NULL DEFAULT 0 CHECK(notified_sequence>=0),
 notified_report jsonb NOT NULL DEFAULT '{}'::jsonb,
 pending_alert boolean NOT NULL DEFAULT false,
 received_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(collector_id,instance_id),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id)
);
CREATE INDEX collector_delivery_enterprise ON collector_delivery_reports(enterprise_id,collector_id,received_at DESC);
CREATE INDEX collector_delivery_outbox ON collector_delivery_reports(received_at,id) WHERE pending_alert;
CREATE INDEX collector_delivery_task_lookup ON runtime_tasks(enterprise_id,(payload->>'report_id'),updated_at) WHERE queue='alert.collector_delivery';
CREATE TABLE collector_delivery_policies (
 policy_id uuid PRIMARY KEY REFERENCES alert_configurations(id),
 collector_id uuid UNIQUE NOT NULL REFERENCES collector_instances(id),
 enterprise_id uuid NOT NULL REFERENCES enterprises(id)
);
-- +goose StatementBegin
DO $$ DECLARE role_name text; BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_telemetry_ingest'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON collector_delivery_reports TO %I',role_name);
   EXECUTE format('GRANT SELECT ON collector_delivery_policies TO %I',role_name);
  END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_worker') THEN
  GRANT INSERT ON collector_delivery_policies TO argus_worker;
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control','kubernetes_operation','monitoring_assignment_retire','alert.collector_delivery'));
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM runtime_tasks WHERE queue='alert.collector_delivery') OR EXISTS(SELECT 1 FROM collector_delivery_reports WHERE sequence>0) THEN
  RAISE EXCEPTION 'Delivery evidence exists; restore a verified snapshot instead of deleting report or task history';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control','kubernetes_operation','monitoring_assignment_retire'));
DROP INDEX collector_delivery_task_lookup;
DROP TABLE collector_delivery_policies;
DROP TABLE collector_delivery_reports;
