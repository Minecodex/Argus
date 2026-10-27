-- +goose Up
ALTER TABLE monitoring_collections ADD COLUMN effective_collector_id uuid,
 ADD COLUMN effective_assignment_epoch bigint NOT NULL DEFAULT 0 CHECK(effective_assignment_epoch>=0),
 ADD CONSTRAINT monitoring_effective_collector FOREIGN KEY(effective_collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id);
UPDATE monitoring_collections c SET effective_collector_id=r.collector_id,effective_assignment_epoch=r.assignment_epoch
FROM monitoring_collection_revisions r WHERE r.enterprise_id=c.enterprise_id AND r.collection_id=c.id AND r.revision=c.effective_revision;
UPDATE monitoring_collections SET effective_collector_id=collector_id,effective_assignment_epoch=assignment_epoch
WHERE effective_revision>0 AND effective_collector_id IS NULL;

ALTER TABLE monitoring_collection_assignments ADD COLUMN activated_at timestamptz,
 ADD COLUMN approved_action_id uuid REFERENCES pending_actions(id);
UPDATE monitoring_collection_assignments SET activated_at=valid_from;
DROP INDEX monitoring_assignment_primary;
CREATE UNIQUE INDEX monitoring_assignment_primary ON monitoring_collection_assignments(collection_id)
WHERE valid_to IS NULL AND activated_at IS NOT NULL;
UPDATE monitoring_collection_assignments a SET valid_to=GREATEST(now(),a.valid_from)
FROM monitoring_collections c WHERE a.enterprise_id=c.enterprise_id AND a.collection_id=c.id
 AND a.valid_to IS NULL AND c.effective_collector_id IS NOT NULL
 AND (a.epoch<>c.effective_assignment_epoch OR a.collector_id<>c.effective_collector_id);
UPDATE monitoring_collection_assignments a SET valid_to=NULL,activated_at=COALESCE(a.activated_at,a.valid_from)
FROM monitoring_collections c WHERE a.enterprise_id=c.enterprise_id AND a.collection_id=c.id
 AND a.epoch=c.effective_assignment_epoch AND a.collector_id=c.effective_collector_id;
UPDATE monitoring_collection_assignments a SET activated_at=NULL
FROM monitoring_collections c WHERE a.enterprise_id=c.enterprise_id AND a.collection_id=c.id
 AND a.valid_to IS NULL AND c.effective_collector_id IS NULL;

CREATE TABLE monitoring_assignment_retirements (
 id uuid PRIMARY KEY,enterprise_id uuid NOT NULL,collection_id uuid NOT NULL,assignment_epoch bigint NOT NULL,
 previous_collector_id uuid NOT NULL,approved_action_id uuid NOT NULL REFERENCES pending_actions(id),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','dispatched','completed','blocked')),
 error_code text NOT NULL DEFAULT '',collector_operation_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id),
 FOREIGN KEY(previous_collector_id,enterprise_id) REFERENCES collector_instances(id,enterprise_id),
 UNIQUE(collection_id,assignment_epoch,previous_collector_id)
);
-- +goose StatementBegin
DO $$ DECLARE role_name text;BEGIN
 FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_connector_gateway'] LOOP
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON monitoring_assignment_retirements TO %I',role_name);
 END IF;END LOOP;END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM monitoring_collection_assignments WHERE activated_at IS NULL AND valid_to IS NULL)
 OR EXISTS(SELECT 1 FROM monitoring_assignment_retirements WHERE status IN ('pending','dispatched','blocked')) THEN
  RAISE EXCEPTION 'Pending collection execution transitions require a verified restore, not a destructive downgrade';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE monitoring_assignment_retirements;
DROP INDEX monitoring_assignment_primary;
CREATE UNIQUE INDEX monitoring_assignment_primary ON monitoring_collection_assignments(collection_id) WHERE valid_to IS NULL;
ALTER TABLE monitoring_collection_assignments DROP COLUMN activated_at,DROP COLUMN approved_action_id;
ALTER TABLE monitoring_collections DROP COLUMN effective_collector_id,DROP COLUMN effective_assignment_epoch;
