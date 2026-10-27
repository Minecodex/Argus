-- +goose Up
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control','kubernetes_operation'));

-- +goose Down
-- Preserve operation evidence; a downgrade requires explicitly retiring owned tasks.
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control'));
