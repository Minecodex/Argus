-- +goose Up
CREATE TABLE dashboard_conversation_contexts (
 conversation_id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
 version bigint NOT NULL CHECK(version>0), selection jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(conversation_id,enterprise_id) REFERENCES conversations(id,enterprise_id)
);
CREATE TABLE dashboard_run_contexts (
 run_id uuid PRIMARY KEY REFERENCES runs(id), enterprise_id uuid NOT NULL, conversation_id uuid NOT NULL,
 owner_user_id uuid NOT NULL REFERENCES enterprise_users(id), context_version bigint NOT NULL,
 selection jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(conversation_id,enterprise_id) REFERENCES conversations(id,enterprise_id)
);
CREATE TRIGGER dashboard_run_context_immutable BEFORE UPDATE ON dashboard_run_contexts FOR EACH ROW EXECUTE FUNCTION reject_dashboard_revision_update();
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON dashboard_conversation_contexts TO %I',role_name);
   EXECUTE format('GRANT SELECT,INSERT ON dashboard_run_contexts TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd
-- +goose Down
DROP TABLE dashboard_run_contexts;
DROP TABLE dashboard_conversation_contexts;
