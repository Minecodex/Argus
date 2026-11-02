-- TenantSchemaManager materializes the trusted metric assignment epoch under
-- the tenant advisory lock. Raw metrics and source identities are retained.
INSERT INTO argus_telemetry.schema_versions (version)
SELECT 6 WHERE NOT EXISTS (SELECT 1 FROM argus_telemetry.schema_versions WHERE version=6);
