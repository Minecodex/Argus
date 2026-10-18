-- +goose Up
CREATE TABLE workspace_result_artifacts (
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    artifact_id uuid NOT NULL,
    path text NOT NULL,
    metadata jsonb NOT NULL CHECK (jsonb_typeof(metadata) = 'object'),
    workspace_file_id uuid REFERENCES workspace_files(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, artifact_id),
    UNIQUE (workspace_id, path)
);
CREATE INDEX workspace_result_artifacts_tenant ON workspace_result_artifacts(enterprise_id,workspace_id);
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON workspace_result_artifacts TO %I',role_name);
    END IF;
  END LOOP;
END
$roles$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE workspace_result_artifacts;
