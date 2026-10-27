package telemetry

import (
	"context"
	"fmt"
)

const numericSampleProjection = "argus_numeric_source_time"

// The raw samples and their identity remain authoritative. This alternate
// physical ordering lets source-scoped, recent numeric queries avoid reading
// almost every UUID granule. Histograms continue to use the complete raw table.
func (m ClickHouseTenantSchemaManager) upgradeSampleProjection(ctx context.Context, tables TenantTables) error {
	table := "`" + tables.MetricSamples + "`"
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" MODIFY SETTING deduplicate_merge_projection_mode='rebuild'"); err != nil {
		return err
	}
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" ADD PROJECTION IF NOT EXISTS "+numericSampleProjection+` (
SELECT series_id,timestamp,float_value,stale_marker,sample_type,source_id,source_revision,source_key,kafka_partition,kafka_offset,record_sequence,ingest_key
ORDER BY (source_key,timestamp,series_id,ingest_key))`); err != nil {
		return err
	}
	// Existing observations must get the same access path. Synchronous mutation
	// completion keeps a partially built projection from being marked ready.
	return m.Conn.Exec(ctx, "ALTER TABLE "+table+" MATERIALIZE PROJECTION "+numericSampleProjection+" SETTINGS mutations_sync=1")
}

func (m ClickHouseTenantSchemaManager) verifySampleProjection(ctx context.Context, tables TenantTables) error {
	var count uint64
	err := m.Conn.QueryRow(ctx, "SELECT count() FROM system.projections WHERE database=currentDatabase() AND table=? AND name=?", tables.MetricSamples, numericSampleProjection).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("telemetry numeric source/time projection is missing")
	}
	return nil
}
