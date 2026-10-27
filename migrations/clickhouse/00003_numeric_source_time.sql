-- TenantSchemaManager adds and materializes the numeric source/time projection
-- under the tenant advisory lock. Raw samples and collection identities remain.
INSERT INTO argus_telemetry.schema_versions (version)
SELECT 5 WHERE NOT EXISTS (SELECT 1 FROM argus_telemetry.schema_versions WHERE version=5);
