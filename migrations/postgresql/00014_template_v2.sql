-- +goose Up
ALTER TABLE template_assets DROP CONSTRAINT template_assets_runtime_check;
ALTER TABLE template_assets ADD CONSTRAINT template_assets_runtime_check CHECK (runtime IN ('argus-template/v1','argus-template/v2'));
ALTER TABLE tool_presentations ADD COLUMN read_operations text[] NOT NULL DEFAULT '{}';
ALTER TABLE tool_presentations ADD COLUMN result_binding jsonb NOT NULL DEFAULT '{}';
CREATE TABLE tool_presentation_snapshots (
    presentation_id uuid PRIMARY KEY REFERENCES tool_presentations(id) ON DELETE CASCADE,
    enterprise_id uuid NOT NULL,
    source_version text NOT NULL,
    data jsonb NOT NULL CHECK (octet_length(data::text)<=16777216),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tool_presentations_query_binding_idx ON tool_presentations ((result_binding->>'job_id')) WHERE result_binding->>'kind'='dashboard_query';
-- +goose StatementBegin
DO $roles$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='argus_server') THEN
        GRANT SELECT ON tool_presentation_snapshots TO argus_server;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='argus_worker') THEN
        GRANT SELECT,INSERT ON tool_presentation_snapshots TO argus_worker;
    END IF;
END; $roles$;
-- +goose StatementEnd
-- +goose Down
DROP TABLE tool_presentation_snapshots;
DROP INDEX tool_presentations_query_binding_idx;
ALTER TABLE tool_presentations DROP COLUMN result_binding;
ALTER TABLE tool_presentations DROP COLUMN read_operations;
-- v2 assets intentionally cannot be downgraded into a v1-only runtime.
