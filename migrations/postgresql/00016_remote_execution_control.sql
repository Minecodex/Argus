-- +goose Up
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution','remote_execution_control'));
UPDATE runtime_tasks SET queue='remote_execution_control' WHERE queue='remote_execution' AND status='pending' AND payload->>'operation'='stop';

-- +goose Down
UPDATE runtime_tasks SET queue='remote_execution' WHERE queue='remote_execution_control';
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query','alert-evaluation','alert-notification','remote_execution'));
