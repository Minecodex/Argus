-- +goose Up
CREATE TABLE monitoring_applications (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 application_key text NOT NULL CHECK (length(application_key) BETWEEN 1 AND 100),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
 environment text NOT NULL CHECK (environment ~ '^[a-z][a-z0-9_-]{0,63}$'),
 description text NOT NULL DEFAULT '', owner_name text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(enterprise_id,application_key), UNIQUE(id,enterprise_id)
);

CREATE TABLE monitoring_application_services (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, application_id uuid NOT NULL,
 service_namespace text NOT NULL DEFAULT '' CHECK(length(service_namespace)<=200),
 service_name text NOT NULL CHECK(length(btrim(service_name)) BETWEEN 1 AND 200),
 runtime text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(application_id,enterprise_id) REFERENCES monitoring_applications(id,enterprise_id),
 UNIQUE(application_id,service_namespace,service_name), UNIQUE(id,enterprise_id), UNIQUE(id,application_id,enterprise_id)
);

CREATE TABLE monitoring_runtime_instances (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, service_id uuid NOT NULL,
 instance_key text NOT NULL CHECK(length(instance_key) BETWEEN 1 AND 512),
 placement jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(placement)='object'),
 first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(service_id,enterprise_id) REFERENCES monitoring_application_services(id,enterprise_id),
 UNIQUE(service_id,instance_key), UNIQUE(id,enterprise_id)
);

CREATE TABLE monitoring_targets (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 kind text NOT NULL CHECK(kind IN ('database','cache','messaging','web','runtime','storage','probe')),
 engine text NOT NULL CHECK(length(engine) BETWEEN 1 AND 64),
 identity_config jsonb NOT NULL CHECK(jsonb_typeof(identity_config)='object'),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,enterprise_id)
);
CREATE INDEX monitoring_targets_catalog ON monitoring_targets(enterprise_id,engine,status,id);

CREATE UNIQUE INDEX monitoring_cluster_enterprise ON kubernetes_clusters(id,enterprise_id);
CREATE UNIQUE INDEX monitoring_collector_enterprise ON collector_instances(id,enterprise_id);
CREATE UNIQUE INDEX monitoring_credential_enterprise ON credentials(id,enterprise_id);
CREATE UNIQUE INDEX monitoring_secret_enterprise ON secrets(id,enterprise_id);

CREATE TABLE kubernetes_namespace_resources (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, cluster_id uuid NOT NULL,
 namespace_uid text NOT NULL CHECK(length(namespace_uid) BETWEEN 1 AND 128),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 253),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','deleted')),
 resource_version text NOT NULL DEFAULT '', observed_at timestamptz NOT NULL DEFAULT now(),
 version bigint NOT NULL DEFAULT 1,
 FOREIGN KEY(cluster_id,enterprise_id) REFERENCES kubernetes_clusters(id,enterprise_id),
 UNIQUE(cluster_id,namespace_uid), UNIQUE(id,enterprise_id)
);
CREATE UNIQUE INDEX kubernetes_namespace_active_name ON kubernetes_namespace_resources(cluster_id,name) WHERE status='active';

CREATE TABLE monitoring_application_bindings (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, application_id uuid NOT NULL, service_id uuid,
 target_type text NOT NULL CHECK(target_type IN ('host','kubernetes_cluster','monitoring_target')),
 target_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1,
 valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz,
 FOREIGN KEY(application_id,enterprise_id) REFERENCES monitoring_applications(id,enterprise_id),
 FOREIGN KEY(service_id,application_id,enterprise_id) REFERENCES monitoring_application_services(id,application_id,enterprise_id),
 CHECK(valid_to IS NULL OR valid_to>=valid_from), UNIQUE(id,enterprise_id)
);
CREATE UNIQUE INDEX monitoring_binding_current ON monitoring_application_bindings(application_id,COALESCE(service_id,'00000000-0000-0000-0000-000000000000'::uuid),target_type,target_id) WHERE valid_to IS NULL;
CREATE INDEX monitoring_binding_target ON monitoring_application_bindings(enterprise_id,target_type,target_id) WHERE valid_to IS NULL;

CREATE TABLE monitoring_collections (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 integration_key text NOT NULL, integration_version text NOT NULL DEFAULT 'v1',
 target_type text NOT NULL CHECK(target_type IN ('host','kubernetes_cluster','monitoring_target','application')),
 target_id uuid NOT NULL, collector_id uuid,
 application_id uuid, service_id uuid,
 config jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(config)='object'),
 credential_id uuid,
 desired_enabled boolean NOT NULL DEFAULT true,
 desired_revision bigint NOT NULL DEFAULT 1 CHECK(desired_revision>0),
 effective_revision bigint NOT NULL DEFAULT 0 CHECK(effective_revision>=0 AND effective_revision<=desired_revision),
 assignment_epoch bigint NOT NULL DEFAULT 1 CHECK(assignment_epoch>0),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 apply_status text NOT NULL DEFAULT 'pending' CHECK(apply_status IN ('pending','applying','effective','failed','unknown')),
 health text NOT NULL DEFAULT 'unknown' CHECK(health IN ('unknown','healthy','degraded','failed','stopped')),
 last_attempt_at timestamptz, last_sample_at timestamptz, observed_at timestamptz,
 error_code text NOT NULL DEFAULT '', error_message text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 FOREIGN KEY(application_id,enterprise_id) REFERENCES monitoring_applications(id,enterprise_id),
 FOREIGN KEY(service_id,application_id,enterprise_id) REFERENCES monitoring_application_services(id,application_id,enterprise_id),
 FOREIGN KEY(credential_id,enterprise_id) REFERENCES credentials(id,enterprise_id),
 CHECK(service_id IS NULL OR application_id IS NOT NULL), UNIQUE(id,enterprise_id)
);
CREATE INDEX monitoring_collection_catalog ON monitoring_collections(enterprise_id,integration_key,status,id);
CREATE INDEX monitoring_collection_executor ON monitoring_collections(collector_id,status);

CREATE TABLE monitoring_collection_revisions (
 collection_id uuid NOT NULL, enterprise_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 integration_version text NOT NULL, config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'),
 desired_enabled boolean NOT NULL, collector_id uuid, credential_id uuid, credential_version bigint,
 assignment_epoch bigint NOT NULL, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(collection_id,revision),
 FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 FOREIGN KEY(credential_id,enterprise_id) REFERENCES credentials(id,enterprise_id),
 CHECK((credential_id IS NULL)=(credential_version IS NULL))
);

CREATE TABLE monitoring_collection_assignments (
 collection_id uuid NOT NULL, enterprise_id uuid NOT NULL, epoch bigint NOT NULL CHECK(epoch>0),
 collector_id uuid NOT NULL, valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz,
 PRIMARY KEY(collection_id,epoch),
 FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 CHECK(valid_to IS NULL OR valid_to>=valid_from)
);
CREATE UNIQUE INDEX monitoring_assignment_primary ON monitoring_collection_assignments(collection_id) WHERE valid_to IS NULL;

CREATE TABLE monitoring_ingest_endpoints (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, application_id uuid NOT NULL,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 protocol text NOT NULL CHECK(protocol IN ('otlp','jaeger','skywalking')),
 signals text[] NOT NULL CHECK(cardinality(signals)>0 AND signals<@ARRAY['metrics','logs','traces']::text[]),
 collector_id uuid, service_allowlist uuid[] NOT NULL DEFAULT '{}',
 enabled boolean NOT NULL DEFAULT true, version bigint NOT NULL DEFAULT 1,
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(application_id,enterprise_id) REFERENCES monitoring_applications(id,enterprise_id),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 UNIQUE(id,enterprise_id), UNIQUE(id,application_id,enterprise_id)
);

CREATE TABLE monitoring_ingest_keys (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL, endpoint_id uuid NOT NULL, application_id uuid NOT NULL,
 key_prefix text NOT NULL, key_digest bytea NOT NULL CHECK(octet_length(key_digest)=32),
 expires_at timestamptz, revoked_at timestamptz, last_used_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(endpoint_id,application_id,enterprise_id) REFERENCES monitoring_ingest_endpoints(id,application_id,enterprise_id),
 UNIQUE(key_digest), UNIQUE(id,enterprise_id)
);

ALTER TABLE secrets DROP CONSTRAINT secrets_owner_type_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_owner_type_check CHECK(owner_type IN ('user','mcp_connection','monitoring_collection','monitoring_draft'));
ALTER TABLE secrets ADD COLUMN purpose text NOT NULL DEFAULT 'platform' CHECK(purpose IN ('platform','monitoring'));
ALTER TABLE secrets ADD CONSTRAINT secrets_monitoring_owner CHECK(
 (owner_type IN ('monitoring_collection','monitoring_draft') AND purpose='monitoring' AND owner_id IS NOT NULL)
 OR (owner_type IN ('user','mcp_connection') AND purpose='platform')
);
ALTER TABLE credentials DROP CONSTRAINT credentials_protocol_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification','monitoring'));

CREATE TABLE monitoring_credential_drafts (
 id uuid PRIMARY KEY, enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 editor_type text NOT NULL CHECK(editor_type IN ('user','service_account')), editor_id uuid NOT NULL,
 collector_id uuid NOT NULL, secret_id uuid NOT NULL,
 target_hash bytea NOT NULL CHECK(octet_length(target_hash)=32),
 expires_at timestamptz NOT NULL, consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 FOREIGN KEY(secret_id,enterprise_id) REFERENCES secrets(id,enterprise_id), UNIQUE(id,enterprise_id)
);

ALTER TABLE data_authorization_grants DROP CONSTRAINT data_authorization_grants_resource_type_check;
ALTER TABLE data_authorization_grants ADD CONSTRAINT data_authorization_grants_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster','dashboard','application','monitoring_target','kubernetes_namespace'));

-- +goose StatementBegin
CREATE FUNCTION validate_monitoring_target_reference() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target_type='host' AND NOT EXISTS(SELECT 1 FROM hosts WHERE id=NEW.target_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'monitoring host outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.target_type='kubernetes_cluster' AND NOT EXISTS(SELECT 1 FROM kubernetes_clusters WHERE id=NEW.target_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'monitoring cluster outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.target_type='monitoring_target' AND NOT EXISTS(SELECT 1 FROM monitoring_targets WHERE id=NEW.target_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'monitoring target outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.target_type='application' AND NOT EXISTS(SELECT 1 FROM monitoring_applications WHERE id=NEW.target_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'monitoring application outside enterprise' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_collection_target BEFORE INSERT OR UPDATE ON monitoring_collections FOR EACH ROW EXECUTE FUNCTION validate_monitoring_target_reference();
CREATE TRIGGER monitoring_binding_target BEFORE INSERT OR UPDATE ON monitoring_application_bindings FOR EACH ROW EXECUTE FUNCTION validate_monitoring_target_reference();

-- +goose StatementBegin
CREATE FUNCTION validate_monitoring_grant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.resource_type='application' AND NOT EXISTS(SELECT 1 FROM monitoring_applications WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'application grant outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.resource_type='monitoring_target' AND NOT EXISTS(SELECT 1 FROM monitoring_targets WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'monitoring target grant outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.resource_type='kubernetes_namespace' AND NOT EXISTS(SELECT 1 FROM kubernetes_namespace_resources WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'namespace grant outside enterprise' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_grant_enterprise_check BEFORE INSERT OR UPDATE ON data_authorization_grants FOR EACH ROW EXECUTE FUNCTION validate_monitoring_grant();

-- +goose StatementBegin
CREATE FUNCTION protect_monitoring_application_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.enterprise_id<>OLD.enterprise_id OR NEW.environment<>OLD.environment OR NEW.application_key<>OLD.application_key THEN
  RAISE EXCEPTION 'application environment and identity are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_application_identity BEFORE UPDATE ON monitoring_applications FOR EACH ROW EXECUTE FUNCTION protect_monitoring_application_identity();
CREATE TRIGGER monitoring_revision_immutable BEFORE UPDATE ON monitoring_collection_revisions FOR EACH ROW EXECUTE FUNCTION reject_dashboard_revision_update();

-- +goose StatementBegin
DO $roles$
DECLARE role_name text; table_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
   FOREACH table_name IN ARRAY ARRAY['monitoring_applications','monitoring_application_services','monitoring_runtime_instances','monitoring_targets','kubernetes_namespace_resources','monitoring_application_bindings','monitoring_collections','monitoring_collection_revisions','monitoring_collection_assignments','monitoring_ingest_endpoints','monitoring_ingest_keys','monitoring_credential_drafts'] LOOP
    EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON %I TO %I',table_name,role_name);
   END LOOP;
  END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_ingest') THEN
  GRANT SELECT ON monitoring_applications,monitoring_targets,monitoring_collections,monitoring_collection_revisions,monitoring_collection_assignments,monitoring_ingest_endpoints,monitoring_ingest_keys TO argus_telemetry_ingest;
  GRANT SELECT,INSERT,UPDATE ON monitoring_application_services,monitoring_runtime_instances TO argus_telemetry_ingest;
  GRANT UPDATE(last_used_at) ON monitoring_ingest_keys TO argus_telemetry_ingest;
 END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='argus_telemetry_query') THEN
  GRANT SELECT ON monitoring_applications,monitoring_application_services,monitoring_runtime_instances,monitoring_targets,kubernetes_namespace_resources,monitoring_application_bindings,monitoring_collections,monitoring_collection_revisions,monitoring_collection_assignments TO argus_telemetry_query;
 END IF;
END $roles$;
-- +goose StatementEnd

-- +goose Down
-- A downgrade must not silently destroy newly created applications or secrets.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM monitoring_applications) OR EXISTS(SELECT 1 FROM monitoring_collections)
 OR EXISTS(SELECT 1 FROM secrets WHERE purpose='monitoring') OR EXISTS(SELECT 1 FROM kubernetes_namespace_resources)
 OR EXISTS(SELECT 1 FROM monitoring_targets) THEN
  RAISE EXCEPTION 'Plan8 data exists; restore a verified backup rather than dropping business data';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER monitoring_grant_enterprise_check ON data_authorization_grants;
DROP FUNCTION validate_monitoring_grant();
DROP TABLE monitoring_credential_drafts,monitoring_ingest_keys,monitoring_ingest_endpoints;
DROP TABLE monitoring_collection_assignments,monitoring_collection_revisions,monitoring_collections,monitoring_application_bindings;
DROP FUNCTION validate_monitoring_target_reference();
DROP TABLE monitoring_runtime_instances,monitoring_application_services,monitoring_applications,monitoring_targets,kubernetes_namespace_resources;
DROP FUNCTION protect_monitoring_application_identity();
ALTER TABLE secrets DROP CONSTRAINT secrets_monitoring_owner;
ALTER TABLE secrets DROP COLUMN purpose;
ALTER TABLE secrets DROP CONSTRAINT secrets_owner_type_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_owner_type_check CHECK(owner_type IN ('user','mcp_connection'));
ALTER TABLE credentials DROP CONSTRAINT credentials_protocol_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_protocol_check CHECK(protocol IN ('ssh','windows','kubernetes','http','notification'));
ALTER TABLE data_authorization_grants DROP CONSTRAINT data_authorization_grants_resource_type_check;
ALTER TABLE data_authorization_grants ADD CONSTRAINT data_authorization_grants_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster','dashboard'));
DROP INDEX monitoring_cluster_enterprise,monitoring_collector_enterprise,monitoring_credential_enterprise,monitoring_secret_enterprise;
