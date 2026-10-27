-- +goose Up
CREATE INDEX telemetry_sources_namespace_lookup ON telemetry_sources(enterprise_id,namespace_uid,collector_id)
 WHERE namespace_uid<>'' AND collection_id IS NOT NULL;

-- +goose Down
DROP INDEX telemetry_sources_namespace_lookup;
