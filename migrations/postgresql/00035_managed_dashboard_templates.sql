-- +goose Up
CREATE TABLE dashboard_template_versions (
 template_key text NOT NULL CHECK(length(template_key) BETWEEN 1 AND 120),
 version bigint NOT NULL CHECK(version>0),runtime_version integer NOT NULL CHECK(runtime_version>0),
 name text NOT NULL,description text NOT NULL DEFAULT '',integration_keys text[] NOT NULL,
 spec jsonb NOT NULL,spec_hash text NOT NULL CHECK(length(spec_hash)=64),
 references_json jsonb NOT NULL DEFAULT '[]',created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(template_key,version)
);
CREATE TABLE dashboard_template_heads (
 template_key text PRIMARY KEY,version bigint NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(template_key,version) REFERENCES dashboard_template_versions(template_key,version)
);
CREATE TABLE managed_dashboard_instances (
 enterprise_id uuid NOT NULL REFERENCES enterprises(id),template_key text NOT NULL,
 dashboard_id uuid NOT NULL,applied_version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(enterprise_id,template_key),UNIQUE(dashboard_id,enterprise_id),
 FOREIGN KEY(dashboard_id,enterprise_id) REFERENCES dashboards(id,enterprise_id),
 FOREIGN KEY(template_key,applied_version) REFERENCES dashboard_template_versions(template_key,version)
);
ALTER TABLE dashboard_drafts ADD COLUMN source_template_key text;
ALTER TABLE dashboard_drafts ADD COLUMN source_template_version bigint;
ALTER TABLE dashboard_drafts ADD CONSTRAINT dashboard_draft_template_origin FOREIGN KEY(source_template_key,source_template_version)
 REFERENCES dashboard_template_versions(template_key,version);
ALTER TABLE dashboard_drafts ADD CONSTRAINT dashboard_draft_template_origin_pair CHECK((source_template_key IS NULL)=(source_template_version IS NULL));

-- +goose StatementBegin
CREATE FUNCTION protect_dashboard_template_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'dashboard template versions are immutable';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER dashboard_template_version_immutable BEFORE UPDATE OR DELETE ON dashboard_template_versions
 FOR EACH ROW EXECUTE FUNCTION protect_dashboard_template_version();

-- +goose StatementBegin
CREATE FUNCTION protect_managed_dashboard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE protected_id uuid; protected_enterprise uuid;
BEGIN
 IF TG_TABLE_NAME='dashboard_revisions' THEN protected_id:=NEW.dashboard_id;protected_enterprise:=NEW.enterprise_id;
 ELSE protected_id:=OLD.id;protected_enterprise:=OLD.enterprise_id;END IF;
 IF EXISTS(SELECT 1 FROM managed_dashboard_instances instance WHERE instance.dashboard_id=protected_id AND instance.enterprise_id=protected_enterprise)
 AND current_setting('argus.managed_dashboard_write',true) IS DISTINCT FROM protected_id::text THEN
   RAISE EXCEPTION 'managed dashboard definition is read only';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER managed_dashboard_read_only BEFORE UPDATE OR DELETE ON dashboards FOR EACH ROW EXECUTE FUNCTION protect_managed_dashboard();
CREATE TRIGGER managed_dashboard_revision_read_only BEFORE INSERT ON dashboard_revisions FOR EACH ROW EXECUTE FUNCTION protect_managed_dashboard();

-- +goose Down
DROP TRIGGER managed_dashboard_revision_read_only ON dashboard_revisions;
DROP TRIGGER managed_dashboard_read_only ON dashboards;
DROP FUNCTION protect_managed_dashboard();
ALTER TABLE dashboard_drafts DROP CONSTRAINT dashboard_draft_template_origin_pair;
ALTER TABLE dashboard_drafts DROP CONSTRAINT dashboard_draft_template_origin;
ALTER TABLE dashboard_drafts DROP COLUMN source_template_version;
ALTER TABLE dashboard_drafts DROP COLUMN source_template_key;
DROP TABLE managed_dashboard_instances;
DROP TABLE dashboard_template_heads;
DROP TRIGGER dashboard_template_version_immutable ON dashboard_template_versions;
DROP FUNCTION protect_dashboard_template_version();
DROP TABLE dashboard_template_versions;
