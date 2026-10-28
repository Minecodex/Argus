package telemetry

import (
	"context"
	"encoding/json"
)

// Unlike the dashboard's name picker, the observation directory must not merge
// equal names with different source/type/unit signatures.
func (q ClickHouseQuery) discoverMetricVariants(ctx context.Context, in DataCatalogRequest, read string, args []any, after string, out *DataCatalogResult) error {
	key := "toJSONString(tuple(metric_name,source_type,metric_type,temporality,toString(is_monotonic),unit,source_key))"
	if in.Search != "" {
		read += " AND positionCaseInsensitive(metric_name,?)>0"
		args = append(args, in.Search)
	}
	if after != "" {
		var parts []string
		if json.Unmarshal([]byte(after), &parts) != nil || len(parts) != 7 {
			return ErrQueryInvalid
		}
		read += " AND " + key + ">?"
		args = append(args, after)
	}
	rows, err := q.Conn.Query(ctx, "SELECT "+key+",metric_name,source_type,concat(metric_type,':',temporality,':',toString(is_monotonic)),unit,arraySort(arrayDistinct(arrayFlatten(groupArray(mapKeys(labels))))),arraySort(groupUniqArray(source_key)) FROM "+read+" GROUP BY metric_name,source_type,metric_type,temporality,is_monotonic,unit,source_key ORDER BY "+key+" LIMIT ?", append(args, in.Limit+1)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	last := ""
	for rows.Next() {
		var item MetricDescriptor
		var cursor, kind string
		if err = rows.Scan(&cursor, &item.Name, &item.SourceType, &kind, &item.Unit, &item.Labels, &item.SourceKeys); err != nil {
			return err
		}
		if len(out.Metrics) == in.Limit {
			out.HasMore = true
			out.Complete = false
			out.NextCursor = writeCatalogCursor(in, last)
			break
		}
		item.Type = metricDescriptorType([]string{kind})
		out.Metrics = append(out.Metrics, item)
		last = cursor
	}
	return rows.Err()
}
