package telemetry

import "context"

// This scalar preserves assignment precedence without reading arbitrary wide
// resource metadata for every series in a ClickHouse granule. Existing raw
// observations remain authoritative and are retained during materialization.
func (m ClickHouseTenantSchemaManager) upgradeMetricAssignmentEpoch(ctx context.Context, tables TenantTables) error {
	table := "`" + tables.MetricSeries + "`"
	if err := m.Conn.Exec(ctx, "ALTER TABLE "+table+" ADD COLUMN IF NOT EXISTS assignment_epoch Int64 MATERIALIZED toInt64OrZero(resource_attributes['argus.assignment.epoch'])"); err != nil {
		return err
	}
	return m.Conn.Exec(ctx, "ALTER TABLE "+table+" MATERIALIZE COLUMN assignment_epoch SETTINGS mutations_sync=1")
}
