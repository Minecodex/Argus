-- Source-aware tenant tables are upgraded by TenantSchemaManager under the
-- tenant advisory lock. Existing data is retained with an unknown origin.
INSERT INTO argus_telemetry.schema_versions (version)
SELECT 4 WHERE NOT EXISTS (SELECT 1 FROM argus_telemetry.schema_versions WHERE version=4);
