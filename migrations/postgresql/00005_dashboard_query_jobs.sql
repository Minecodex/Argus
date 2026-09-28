-- +goose Up
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox','dashboard_query'));

CREATE TABLE dashboard_query_jobs (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 conversation_id uuid NOT NULL REFERENCES conversations(id), owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
 run_id uuid REFERENCES runs(id), dashboard_id uuid NOT NULL, revision_id uuid NOT NULL,
 authorization_version bigint NOT NULL, task_id uuid NOT NULL UNIQUE REFERENCES runtime_tasks(id),
 request_key text NOT NULL, input_hash text NOT NULL, frozen_plan jsonb NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','fetching','materialized','delivering','complete','partial','cancelled','failed')),
 attempt_id uuid, manifest jsonb, error_code text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id),
 FOREIGN KEY(revision_id,enterprise_id) REFERENCES dashboard_revisions(id,enterprise_id),
 UNIQUE(enterprise_id,conversation_id,owner_user_id,request_key), UNIQUE(id,enterprise_id)
);
CREATE INDEX dashboard_query_conversation_idx ON dashboard_query_jobs(enterprise_id,conversation_id,created_at);
CREATE TABLE dashboard_query_attempts (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, job_id uuid NOT NULL,
 ordinal integer NOT NULL, status text NOT NULL CHECK(status IN ('fetching','materialized','abandoned')),
 created_at timestamptz NOT NULL DEFAULT now(), sealed_at timestamptz,
 FOREIGN KEY(job_id,enterprise_id) REFERENCES dashboard_query_jobs(id,enterprise_id),
 UNIQUE(job_id,ordinal), UNIQUE(id,job_id,enterprise_id)
);
ALTER TABLE dashboard_query_jobs ADD CONSTRAINT dashboard_query_attempt_fk FOREIGN KEY(attempt_id,id,enterprise_id) REFERENCES dashboard_query_attempts(id,job_id,enterprise_id);
CREATE TABLE dashboard_query_files (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, job_id uuid NOT NULL, attempt_id uuid NOT NULL,
 panel_id text NOT NULL, target_id text NOT NULL, file_kind text NOT NULL CHECK(file_kind IN ('data','manifest')),
 byte_size bigint NOT NULL CHECK(byte_size>=0), content_hash text NOT NULL, chunks jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(attempt_id,job_id,enterprise_id) REFERENCES dashboard_query_attempts(id,job_id,enterprise_id),
 UNIQUE(attempt_id,panel_id,target_id,file_kind), UNIQUE(id,enterprise_id)
);
CREATE TABLE dashboard_query_deliveries (
 file_id uuid NOT NULL, enterprise_id uuid NOT NULL, workspace_id uuid NOT NULL REFERENCES workspaces(id),
 workspace_file_id uuid NOT NULL REFERENCES workspace_files(id), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(file_id,enterprise_id) REFERENCES dashboard_query_files(id,enterprise_id), PRIMARY KEY(file_id,workspace_id)
);

-- +goose StatementBegin
CREATE FUNCTION protect_dashboard_query_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.enterprise_id,NEW.conversation_id,NEW.owner_user_id,NEW.run_id,NEW.dashboard_id,NEW.revision_id,NEW.authorization_version,NEW.task_id,NEW.request_key,NEW.input_hash,NEW.frozen_plan)
 IS DISTINCT FROM (OLD.enterprise_id,OLD.conversation_id,OLD.owner_user_id,OLD.run_id,OLD.dashboard_id,OLD.revision_id,OLD.authorization_version,OLD.task_id,OLD.request_key,OLD.input_hash,OLD.frozen_plan)
 OR (OLD.manifest IS NOT NULL AND (NEW.manifest IS DISTINCT FROM OLD.manifest OR NEW.attempt_id IS DISTINCT FROM OLD.attempt_id)) THEN
  RAISE EXCEPTION 'dashboard query snapshot is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER dashboard_query_snapshot_immutable BEFORE UPDATE ON dashboard_query_jobs FOR EACH ROW EXECUTE FUNCTION protect_dashboard_query_snapshot();
CREATE TRIGGER dashboard_query_file_immutable BEFORE UPDATE ON dashboard_query_files FOR EACH ROW EXECUTE FUNCTION reject_dashboard_revision_update();

-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON dashboard_query_jobs,dashboard_query_attempts TO %I',role_name);
   EXECUTE format('GRANT SELECT,INSERT ON dashboard_query_files,dashboard_query_deliveries TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE dashboard_query_deliveries;
DROP TABLE dashboard_query_files;
ALTER TABLE dashboard_query_jobs DROP CONSTRAINT dashboard_query_attempt_fk;
DROP TABLE dashboard_query_attempts;
DROP TABLE dashboard_query_jobs;
DROP FUNCTION protect_dashboard_query_snapshot();
DELETE FROM runtime_tasks WHERE queue='dashboard_query';
ALTER TABLE runtime_tasks DROP CONSTRAINT runtime_tasks_queue_check;
ALTER TABLE runtime_tasks ADD CONSTRAINT runtime_tasks_queue_check CHECK(queue IN ('agent','action','compaction','sandbox'));
