package promqlengine

import (
	"fmt"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chscope"
)

// The constant UUID set comes exclusively from the authorized FINAL metadata
// selection, in source-precedence order. For small source scopes an external
// JOIN/subquery prevents MergeTree from using its source/metric/time projection.
// A native constant transform preserves that exact order without a JOIN.
func sampleSelectionQuery(table, columns string, sourceCount int) (string, bool) {
	constantSelection := sourceCount <= 128
	ids := "{argus_selected_ids:Array(UUID)}"
	bounds := []string{
		"timestamp >= toDateTime64(fromUnixTimestamp64Milli({argus_mint:Int64}, 'UTC'),9,'UTC')",
		"timestamp <= toDateTime64(fromUnixTimestamp64Milli({argus_maxt:Int64}, 'UTC'),9,'UTC')",
	}
	prewhere := append([]string{"series_id IN " + ids, "metric_name IN {argus_metric_names:Array(String)}"}, bounds...)
	where := append([]string{"series_id IN " + ids}, bounds...)
	from := quoteIdentifier(table)
	order := "selection_order"
	if constantSelection {
		// UUID transform builds a constant lookup, avoiding a linear indexOf
		// scan for every sample. Values match the former UInt64 join ordinal.
		order = "transform(series_id," + ids + ",range(toUInt64(length(" + ids + "))),toUInt64(0))"
		if sourceCount > 0 {
			predicate := "source_key IN {argus_base_keys:Array(String)}"
			prewhere, where = append(prewhere, predicate), append(where, predicate)
		}
	} else {
		from += " INNER JOIN argus_selected_series ON series_id=selected_id"
		where[0] = "series_id IN (SELECT selected_id FROM argus_selected_series)"
		predicate := chscope.BasePredicate("source_key")
		prewhere, where = append(prewhere, predicate), append(where, predicate)
	}
	return fmt.Sprintf("SELECT %s FROM %s PREWHERE %s WHERE %s ORDER BY timestamp,%s,kafka_partition,kafka_offset,record_sequence,ingest_key",
		columns, from, strings.Join(prewhere, " AND "), strings.Join(where, " AND "), order), constantSelection
}
