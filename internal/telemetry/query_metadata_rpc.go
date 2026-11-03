package telemetry

import (
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
)

func inputMetricsToProto(values []metricmeta.Descriptor) []*telemetryv1.InputMetricDescriptor {
	result := make([]*telemetryv1.InputMetricDescriptor, 0, len(values))
	for _, value := range values {
		result = append(result, &telemetryv1.InputMetricDescriptor{Name: value.Name, Type: value.Type, Unit: value.Unit, StorageType: value.StorageType, Temporality: value.Temporality, Monotonic: value.Monotonic, SourceKey: value.SourceKey})
	}
	return result
}

func inputMetricsFromProto(values []*telemetryv1.InputMetricDescriptor) []metricmeta.Descriptor {
	result := make([]metricmeta.Descriptor, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, metricmeta.Descriptor{Name: value.Name, Type: value.Type, Unit: value.Unit, StorageType: value.StorageType, Temporality: value.Temporality, Monotonic: value.Monotonic, SourceKey: value.SourceKey})
		}
	}
	return result
}
