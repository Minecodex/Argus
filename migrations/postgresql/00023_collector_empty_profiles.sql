-- +goose Up
-- Control identity/health can remain active with every collection disabled.
ALTER TABLE collector_config_revisions DROP CONSTRAINT collector_config_revisions_profile_ids_check;

-- +goose Down
-- Refuse rollback while valid empty-profile revisions exist; do not silently
-- enable a source merely to satisfy the old schema.
ALTER TABLE collector_config_revisions ADD CONSTRAINT collector_config_revisions_profile_ids_check CHECK(cardinality(profile_ids)>0);
