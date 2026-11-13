-- +goose Up
ALTER TABLE dashboard_bindings ADD COLUMN purpose text NOT NULL DEFAULT 'resource'
 CHECK (purpose IN ('resource','overview'));
ALTER TABLE dashboard_bindings ADD CONSTRAINT dashboard_overview_cluster_only
 CHECK (purpose='resource' OR resource_type='kubernetes_cluster');
-- +goose StatementBegin
DO $$
DECLARE existing_name text;
BEGIN
 SELECT conname INTO existing_name FROM pg_constraint
 WHERE conrelid='dashboard_bindings'::regclass AND contype='u'
 AND pg_get_constraintdef(oid)='UNIQUE (enterprise_id, dashboard_id, resource_type, resource_id)';
 IF existing_name IS NOT NULL THEN
  EXECUTE format('ALTER TABLE dashboard_bindings DROP CONSTRAINT %I',existing_name);
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE dashboard_bindings ADD CONSTRAINT dashboard_binding_purpose_unique
 UNIQUE(enterprise_id,dashboard_id,resource_type,resource_id,purpose);
CREATE UNIQUE INDEX dashboard_overview_resource_unique
 ON dashboard_bindings(enterprise_id,resource_type,resource_id) WHERE purpose='overview';

-- +goose Down
DELETE FROM dashboard_bindings WHERE purpose='overview';
DROP INDEX dashboard_overview_resource_unique;
ALTER TABLE dashboard_bindings DROP CONSTRAINT dashboard_binding_purpose_unique;
ALTER TABLE dashboard_bindings DROP CONSTRAINT dashboard_overview_cluster_only;
ALTER TABLE dashboard_bindings DROP COLUMN purpose;
ALTER TABLE dashboard_bindings ADD UNIQUE(enterprise_id,dashboard_id,resource_type,resource_id);
