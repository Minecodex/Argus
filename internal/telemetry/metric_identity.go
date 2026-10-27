package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// Resource dimensions are part of OTel metric identity. In particular, two
// Pods or two service instances may have identical datapoint attributes.
// The fingerprint preserves distinctions even when Prometheus normalization
// makes two attribute names collide. Transport revisions are provenance only.
func addMetricResourceLabels(labels map[string]string, values []*commonpb.KeyValue) {
	attrs := attributesMap(values)
	dimensions := make(map[string]string)
	for key, value := range attrs {
		if strings.HasPrefix(key, "argus.") {
			continue
		}
		dimensions[key] = value
	}
	if len(dimensions) > 0 {
		labels["argus_resource_fingerprint"] = attributeFingerprint(values, true)
		// Readable selectors are namespaced separately from datapoint labels.
		// Colliding normalized names are omitted rather than chosen randomly.
		seen := map[string]string{}
		for key, value := range dimensions {
			name := "resource_" + promLabelName(key)
			if old, exists := seen[name]; exists && old != key {
				delete(labels, name)
				seen[name] = ""
				continue
			}
			seen[name] = key
			labels[name] = value
		}
	}
	if collection := attrs["argus.collection.id"]; collection != "" {
		labels["argus_collection_id"] = collection
	}
}

func metricScopeLabels(labels map[string]string, scope *commonpb.InstrumentationScope) {
	delete(labels, "otel_scope_name")
	delete(labels, "otel_scope_version")
	if scope.GetName() != "" {
		labels["otel_scope_name"] = scope.GetName()
	}
	if scope.GetVersion() != "" {
		labels["otel_scope_version"] = scope.GetVersion()
	}
	if len(scope.GetAttributes()) > 0 {
		labels["argus_scope_fingerprint"] = attributeFingerprint(scope.GetAttributes(), false)
	}
}

func attributeFingerprint(values []*commonpb.KeyValue, excludeTransport bool) string {
	attributes := map[string]*commonpb.AnyValue{}
	for _, value := range values {
		if value != nil && (!excludeTransport || !strings.HasPrefix(value.Key, "argus.")) {
			attributes[value.Key] = value.Value
		}
	}
	raw, _ := json.Marshal(attributes)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
