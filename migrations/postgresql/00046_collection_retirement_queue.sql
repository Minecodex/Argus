-- +goose Up
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control','kubernetes_operation','monitoring_assignment_retire'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM runtime_tasks WHERE queue='monitoring_assignment_retire') THEN
  RAISE EXCEPTION 'Retirement task history exists; restore a verified snapshot rather than dropping task evidence';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control','kubernetes_operation'));
