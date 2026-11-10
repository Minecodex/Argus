package telemetry

import (
	"context"
	"fmt"
	"time"
)

const numericSampleProjection = "argus_numeric_source_metric_time"
const histogramSampleProjection = "argus_histogram_source_metric_time"

// The raw samples and their identity remain authoritative. This alternate
// physical ordering lets source-scoped, recent queries avoid reading almost
// every UUID granule. Keep a compact numeric projection and a complete payload
// projection for classic/native histograms and summaries.
func (m ClickHouseTenantSchemaManager) upgradeSampleProjection(ctx context.Context, tables TenantTables) error {
	table := "`" + tables.MetricSamples + "`"
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" MODIFY SETTING deduplicate_merge_projection_mode='rebuild'"); err != nil {
		return err
	}
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" ADD PROJECTION IF NOT EXISTS "+numericSampleProjection+` (
SELECT series_id,metric_name,timestamp,float_value,stale_marker,sample_type,source_id,source_revision,source_key,kafka_partition,kafka_offset,record_sequence,ingest_key
ORDER BY (source_key,metric_name,timestamp,series_id,ingest_key))`); err != nil {
		return err
	}
	if err := m.materializeSampleProjection(ctx, tables, numericSampleProjection); err != nil {
		return err
	}
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" ADD PROJECTION IF NOT EXISTS "+histogramSampleProjection+` (
SELECT series_id,metric_name,timestamp,float_value,stale_marker,sample_type,count,sum,bucket_counts,explicit_bounds,quantile_values,
exponential_scale,exponential_zero_count,exponential_zero_threshold,exponential_positive_offset,exponential_positive_bucket_counts,
exponential_negative_offset,exponential_negative_bucket_counts,source_id,source_revision,source_key,kafka_partition,kafka_offset,record_sequence,ingest_key
ORDER BY (source_key,metric_name,timestamp,series_id,ingest_key))`); err != nil {
		return err
	}
	return m.materializeSampleProjection(ctx, tables, histogramSampleProjection)
}

func (m ClickHouseTenantSchemaManager) missingProjectionParts(ctx context.Context, tables TenantTables, name string) (uint64, error) {
	var missing uint64
	err := m.Conn.QueryRow(ctx, "SELECT count() FROM system.parts WHERE database=currentDatabase() AND table=? AND active AND NOT has(projections,?)", tables.MetricSamples, name).Scan(&missing)
	return missing, err
}

// Materialization may outlive a TCP read deadline. Submit once, observe short
// reads until all active parts are covered, and resume a pending mutation after
// cancellation/restart rather than rebuilding every projection again.
func (m ClickHouseTenantSchemaManager) materializeSampleProjection(ctx context.Context, tables TenantTables, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	missing, err := m.missingProjectionParts(ctx, tables, name)
	if err != nil || missing == 0 {
		return err
	}
	var pending uint64
	command := "(MATERIALIZE PROJECTION " + name + ")"
	if err := m.Conn.QueryRow(ctx, "SELECT count() FROM system.mutations WHERE database=currentDatabase() AND table=? AND command=? AND NOT is_done", tables.MetricSamples, command).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		if err := m.Conn.Exec(ctx, "ALTER TABLE `"+tables.MetricSamples+"` MATERIALIZE PROJECTION "+name+" SETTINGS mutations_sync=0"); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		missing, err = m.missingProjectionParts(ctx, tables, name)
		if err != nil || missing == 0 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m ClickHouseTenantSchemaManager) verifySampleProjection(ctx context.Context, tables TenantTables) error {
	var count uint64
	err := m.Conn.QueryRow(ctx, "SELECT count() FROM system.projections WHERE database=currentDatabase() AND table=? AND name IN (?,?)", tables.MetricSamples, numericSampleProjection, histogramSampleProjection).Scan(&count)
	if err != nil {
		return err
	}
	if count != 2 {
		return fmt.Errorf("telemetry numeric or histogram source/time projection is missing")
	}
	for _, name := range []string{numericSampleProjection, histogramSampleProjection} {
		missing, err := m.missingProjectionParts(ctx, tables, name)
		if err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("telemetry source/time projection has unmaterialized parts")
		}
	}
	return nil
}
