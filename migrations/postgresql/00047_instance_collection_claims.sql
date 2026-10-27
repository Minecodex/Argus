-- +goose Up
ALTER TABLE collection_claims ADD COLUMN collection_id uuid,ADD COLUMN assignment_epoch bigint,
 ADD COLUMN config_revision bigint,
 ADD CONSTRAINT collection_claim_instance FOREIGN KEY(collection_id,enterprise_id) REFERENCES monitoring_collections(id,enterprise_id);

-- +goose Down
ALTER TABLE collection_claims DROP COLUMN collection_id,DROP COLUMN assignment_epoch,DROP COLUMN config_revision;
