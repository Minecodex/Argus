-- +goose Up
CREATE TABLE dashboard_folders (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120), description text NOT NULL DEFAULT '',
 sort_order integer NOT NULL DEFAULT 0, status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_by uuid NOT NULL, updated_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,enterprise_id)
);
CREATE UNIQUE INDEX dashboard_folders_name_idx ON dashboard_folders(enterprise_id,lower(name)) WHERE status='active';

CREATE TABLE dashboards (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id), folder_id uuid,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120), description text NOT NULL DEFAULT '',
 active_revision_id uuid, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 lifecycle text NOT NULL DEFAULT 'active' CHECK(lifecycle IN ('active','archived')),
 created_by uuid NOT NULL, updated_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,enterprise_id), FOREIGN KEY(folder_id,enterprise_id) REFERENCES dashboard_folders(id,enterprise_id)
);
CREATE INDEX dashboards_directory_idx ON dashboards(enterprise_id,lifecycle,folder_id,name,id);

CREATE TABLE dashboard_revisions (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, dashboard_id uuid NOT NULL,
 revision_number bigint NOT NULL CHECK(revision_number>0), schema_version text NOT NULL,
 name text NOT NULL, description text NOT NULL, folder_id uuid,
 spec jsonb NOT NULL, spec_hash text NOT NULL, validation_report jsonb NOT NULL, sample_report jsonb NOT NULL,
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,enterprise_id), UNIQUE(dashboard_id,revision_number), UNIQUE(id,dashboard_id,enterprise_id),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id)
);
ALTER TABLE dashboards ADD CONSTRAINT dashboards_active_revision_fk
 FOREIGN KEY(active_revision_id,id,enterprise_id) REFERENCES dashboard_revisions(id,dashboard_id,enterprise_id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE dashboard_drafts (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id), dashboard_id uuid,
 editor_subject_type text NOT NULL CHECK(editor_subject_type IN ('user','service_account')), editor_subject_id uuid NOT NULL,
 base_revision_id uuid, base_object_version bigint NOT NULL DEFAULT 0,
 draft_version bigint NOT NULL DEFAULT 1 CHECK(draft_version>0),
 status text NOT NULL DEFAULT 'editing' CHECK(status IN ('editing','published','discarded')),
 published_revision_id uuid, name text NOT NULL, description text NOT NULL DEFAULT '', folder_id uuid,
 spec jsonb NOT NULL, proposed_bindings jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,enterprise_id), FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id),
 FOREIGN KEY(base_revision_id,dashboard_id,enterprise_id) REFERENCES dashboard_revisions(id,dashboard_id,enterprise_id),
 FOREIGN KEY(published_revision_id,dashboard_id,enterprise_id) REFERENCES dashboard_revisions(id,dashboard_id,enterprise_id),
 FOREIGN KEY(folder_id,enterprise_id) REFERENCES dashboard_folders(id,enterprise_id)
);
CREATE UNIQUE INDEX dashboard_personal_draft_idx ON dashboard_drafts(enterprise_id,dashboard_id,editor_subject_type,editor_subject_id) WHERE status='editing' AND dashboard_id IS NOT NULL;
CREATE INDEX dashboard_draft_owner_idx ON dashboard_drafts(enterprise_id,editor_subject_type,editor_subject_id,status,updated_at);

CREATE TABLE dashboard_bindings (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, dashboard_id uuid NOT NULL,
 resource_type text NOT NULL CHECK(resource_type IN ('host','kubernetes_cluster')), resource_id uuid NOT NULL,
 version bigint NOT NULL DEFAULT 1, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id),
 UNIQUE(enterprise_id,dashboard_id,resource_type,resource_id)
);

ALTER TABLE data_authorization_grants DROP CONSTRAINT data_authorization_grants_resource_type_check;
ALTER TABLE data_authorization_grants ADD CONSTRAINT data_authorization_grants_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster','dashboard'));

-- +goose StatementBegin
CREATE FUNCTION validate_dashboard_grant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.resource_type='dashboard' AND NOT EXISTS(SELECT 1 FROM dashboards WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'dashboard does not belong to enterprise' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER dashboard_grant_enterprise_check BEFORE INSERT OR UPDATE ON data_authorization_grants FOR EACH ROW EXECUTE FUNCTION validate_dashboard_grant();

-- +goose StatementBegin
CREATE FUNCTION reject_dashboard_revision_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'published dashboard revisions are immutable' USING ERRCODE='23514'; END; $$;
-- +goose StatementEnd
CREATE TRIGGER dashboard_revision_immutable BEFORE UPDATE ON dashboard_revisions FOR EACH ROW EXECUTE FUNCTION reject_dashboard_revision_update();

INSERT INTO permissions(id,description,registry_version) VALUES
 ('telemetry.dashboard.read','Read authorized dashboards and published query definitions',10),
 ('telemetry.dashboard.manage','Create, edit and publish authorized dashboards',10)
ON CONFLICT(id) DO UPDATE SET description=EXCLUDED.description,registry_version=EXCLUDED.registry_version;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT id,'telemetry.dashboard.read' FROM roles WHERE builtin AND identity_key IN ('enterprise_admin','resource_admin','resource_operator','resource_viewer')
 ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT id,'telemetry.dashboard.manage' FROM roles WHERE builtin AND identity_key IN ('enterprise_admin','resource_admin')
 ON CONFLICT DO NOTHING;

-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON dashboard_folders,dashboards,dashboard_drafts,dashboard_bindings TO %I',role_name);
   EXECUTE format('GRANT SELECT, INSERT ON dashboard_revisions TO %I',role_name);
  END IF;
 END LOOP;
END $roles$;
-- +goose StatementEnd

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN ('telemetry.dashboard.read','telemetry.dashboard.manage');
DELETE FROM permissions WHERE id IN ('telemetry.dashboard.read','telemetry.dashboard.manage');
DROP TRIGGER dashboard_grant_enterprise_check ON data_authorization_grants;
DROP FUNCTION validate_dashboard_grant();
DELETE FROM data_authorization_grants WHERE resource_type='dashboard';
ALTER TABLE data_authorization_grants DROP CONSTRAINT data_authorization_grants_resource_type_check;
ALTER TABLE data_authorization_grants ADD CONSTRAINT data_authorization_grants_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster'));
DROP TABLE dashboard_bindings;
DROP TABLE dashboard_drafts;
ALTER TABLE dashboards DROP CONSTRAINT dashboards_active_revision_fk;
DROP TABLE dashboard_revisions;
DROP FUNCTION reject_dashboard_revision_update();
DROP TABLE dashboards;
DROP TABLE dashboard_folders;
