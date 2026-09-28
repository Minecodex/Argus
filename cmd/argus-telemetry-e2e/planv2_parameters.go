//go:build m4e2e

package main

import (
	"fmt"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// More than both Runtime's 200-value preview and Catalog's 100-value UI page.
func planV2SelectionMetric(now uint64) *metricspb.Metric {
	points := []*metricspb.NumberDataPoint{}
	for _, pool := range []string{"blue", "green"} {
		count := 230
		if pool == "green" {
			count = 20
		}
		for i := 0; i < count; i++ {
			points = append(points, &metricspb.NumberDataPoint{TimeUnixNano: now, Attributes: []*commonpb.KeyValue{{Key: "pool", Value: stringValue(pool)}, {Key: "member", Value: stringValue(fmt.Sprintf("%s-%03d", pool, i))}}, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: float64(i + 1)}})
		}
	}
	return &metricspb.Metric{Name: "argus.planv2.selection", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}}
}

func planV2SelectionLogs(now uint64) []*logspb.LogRecord {
	return []*logspb.LogRecord{
		{TimeUnixNano: now, SeverityText: "INFO", Body: stringValue("planv2 selection blue")},
		{TimeUnixNano: now, SeverityText: "ERROR", Body: stringValue("planv2 selection green")},
	}
}
