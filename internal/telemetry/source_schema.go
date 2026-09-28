package telemetry

import (
	"context"
	"fmt"
)

// New sorting dimensions are added in the same ALTER as their columns. A
// failed migration leaves the tenant unavailable and retains its data.
func (m ClickHouseTenantSchemaManager) upgradeSourceColumns(ctx context.Context, t TenantTables) error {
	for table, expectation := range tenantTableExpectations(t) {
		var count uint64
		if err := m.Conn.QueryRow(ctx, "SELECT count() FROM system.columns WHERE database=currentDatabase() AND table=? AND name='source_id'", table).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			continue
		}
		ddl := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN source_id UUID, ADD COLUMN source_revision Int64, ADD COLUMN source_type LowCardinality(String), ADD COLUMN source_key String MATERIALIZED concat(toString(source_id), ':', toString(source_revision))", table)
		if table == t.Traces || table == t.TraceSummary || table == t.TraceSpanEdges {
			ddl += ", MODIFY ORDER BY (" + expectation.SortingKey + ")"
		}
		if table == t.Logs {
			ddl += ", ADD COLUMN IF NOT EXISTS resource_attributes Map(String,String)"
		}
		if err := m.Conn.Exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}
