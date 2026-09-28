-- +goose Up
CREATE TABLE dashboard_parameter_states (
 enterprise_id uuid NOT NULL, conversation_id uuid NOT NULL, dashboard_id uuid NOT NULL,
 owner_user_id uuid NOT NULL, run_id uuid NOT NULL REFERENCES runs(id),
 version bigint NOT NULL CHECK(version>0), state jsonb NOT NULL,
 PRIMARY KEY(conversation_id,dashboard_id),
 FOREIGN KEY(conversation_id,enterprise_id) REFERENCES conversations(id,enterprise_id),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id)
);
CREATE TABLE dashboard_run_parameters (
 run_id uuid NOT NULL REFERENCES runs(id), enterprise_id uuid NOT NULL, conversation_id uuid NOT NULL,
 dashboard_id uuid NOT NULL, owner_user_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>=0), state jsonb NOT NULL,
 PRIMARY KEY(run_id,dashboard_id),
 FOREIGN KEY(conversation_id,enterprise_id) REFERENCES conversations(id,enterprise_id),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id)
);
CREATE TABLE dashboard_analysis_contexts (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, conversation_id uuid NOT NULL, run_id uuid NOT NULL,
 dashboard_id uuid NOT NULL, owner_user_id uuid NOT NULL, revision_id uuid NOT NULL,
 condition_version bigint NOT NULL, input_hash text NOT NULL, user_event_id uuid NOT NULL,
 state jsonb NOT NULL, parameters jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(run_id,dashboard_id) REFERENCES dashboard_run_parameters(run_id,dashboard_id),
 FOREIGN KEY(revision_id,dashboard_id,enterprise_id) REFERENCES dashboard_revisions(id,dashboard_id,enterprise_id)
);
CREATE TRIGGER dashboard_analysis_context_immutable BEFORE UPDATE ON dashboard_analysis_contexts FOR EACH ROW EXECUTE FUNCTION reject_dashboard_revision_update();
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON dashboard_parameter_states,dashboard_run_parameters TO %I',role_name);
   EXECUTE format('GRANT SELECT,INSERT ON dashboard_analysis_contexts TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd
-- +goose Down
DROP TABLE dashboard_analysis_contexts;
DROP TABLE dashboard_run_parameters;
DROP TABLE dashboard_parameter_states;
