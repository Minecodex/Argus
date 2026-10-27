package telemetry

import (
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func redactMetricPoint(attributes []*commonpb.KeyValue, exemplars []*metricspb.Exemplar) error {
	if err := redactOTLPAttributes(attributes); err != nil {
		return err
	}
	for _, exemplar := range exemplars {
		if exemplar == nil {
			return errIngestRejected
		}
		if err := redactOTLPAttributes(exemplar.FilteredAttributes); err != nil {
			return err
		}
	}
	return nil
}

// Attribute projection leaves measurements, temporality, buckets and exemplar
// trace/span IDs unchanged. It also covers native Prometheus metric labels.
func redactMetricPayload(request *collectmetrics.ExportMetricsServiceRequest) error {
	for _, resource := range request.ResourceMetrics {
		if resource == nil {
			return errIngestRejected
		}
		if resource.Resource != nil {
			if err := redactOTLPAttributes(resource.Resource.Attributes); err != nil {
				return err
			}
		}
		for _, scope := range resource.ScopeMetrics {
			if scope == nil {
				return errIngestRejected
			}
			if scope.Scope != nil {
				if err := redactOTLPAttributes(scope.Scope.Attributes); err != nil {
					return err
				}
			}
			for _, metric := range scope.Metrics {
				if metric == nil {
					return errIngestRejected
				}
				if err := redactOTLPAttributes(metric.Metadata); err != nil {
					return err
				}
				switch data := metric.Data.(type) {
				case *metricspb.Metric_Gauge:
					for _, point := range data.Gauge.DataPoints {
						if point == nil {
							return errIngestRejected
						}
						if err := redactMetricPoint(point.Attributes, point.Exemplars); err != nil {
							return err
						}
					}
				case *metricspb.Metric_Sum:
					for _, point := range data.Sum.DataPoints {
						if point == nil {
							return errIngestRejected
						}
						if err := redactMetricPoint(point.Attributes, point.Exemplars); err != nil {
							return err
						}
					}
				case *metricspb.Metric_Histogram:
					for _, point := range data.Histogram.DataPoints {
						if point == nil {
							return errIngestRejected
						}
						if err := redactMetricPoint(point.Attributes, point.Exemplars); err != nil {
							return err
						}
					}
				case *metricspb.Metric_ExponentialHistogram:
					for _, point := range data.ExponentialHistogram.DataPoints {
						if point == nil {
							return errIngestRejected
						}
						if err := redactMetricPoint(point.Attributes, point.Exemplars); err != nil {
							return err
						}
					}
				case *metricspb.Metric_Summary:
					for _, point := range data.Summary.DataPoints {
						if point == nil {
							return errIngestRejected
						}
						if err := redactMetricPoint(point.Attributes, nil); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
