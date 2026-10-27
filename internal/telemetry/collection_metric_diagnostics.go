package telemetry

import (
	"github.com/kakj-go/Argus/internal/builtincollection"
	"time"

	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type collectionMetricDiagnostic struct {
	health, code string
	hasData      bool
	hasPoint     bool
	hasUp        bool
	observedAt   *time.Time
	dataAt       *time.Time
}

func nativeScrapeDiagnostic(resource *metrics.ResourceMetrics) collectionMetricDiagnostic {
	result := collectionMetricDiagnostic{health: "healthy"}
	source := attributesMap(resource.GetResource().GetAttributes())["argus.source.type"]
	native := builtincollection.ScrapedSource(source)
	up, down := false, false
	var observed, data uint64
	for _, scope := range resource.ScopeMetrics {
		for _, metric := range scope.Metrics {
			check := func(at uint64, flags uint32) {
				if flags&1 != 0 {
					return
				} // OTLP no-recorded-value marker.
				result.hasPoint = true
				observed = max(observed, at)
				if !native || metric.Name != "up" && !hasScrapeDiagnosticPrefix(metric.Name) {
					data = max(data, at)
					result.hasData = true
				}
			}
			for _, point := range metric.GetGauge().GetDataPoints() {
				check(point.TimeUnixNano, point.Flags)
				if native && metric.Name == "up" && point.Flags&1 == 0 {
					if point.GetAsDouble()+float64(point.GetAsInt()) == 0 {
						down = true
					} else {
						up = true
					}
				}
			}
			for _, point := range metric.GetSum().GetDataPoints() {
				check(point.TimeUnixNano, point.Flags)
			}
			for _, point := range metric.GetHistogram().GetDataPoints() {
				check(point.TimeUnixNano, point.Flags)
			}
			for _, point := range metric.GetExponentialHistogram().GetDataPoints() {
				check(point.TimeUnixNano, point.Flags)
			}
			for _, point := range metric.GetSummary().GetDataPoints() {
				check(point.TimeUnixNano, point.Flags)
			}
		}
	}
	if down {
		result.health, result.code = "failed", "COLLECTION_SCRAPE_FAILED"
		if up {
			result.health, result.code = "degraded", "COLLECTION_SCRAPE_PARTIAL_FAILURE"
		}
	}
	result.hasUp = up
	if native && observed > 0 {
		at := time.Unix(0, int64(observed)).UTC()
		result.observedAt = &at
	}
	if native && data > 0 {
		at := time.Unix(0, int64(data)).UTC()
		result.dataAt = &at
	}
	return result
}

func hasScrapeDiagnosticPrefix(name string) bool {
	return len(name) >= 7 && name[:7] == "scrape_"
}
