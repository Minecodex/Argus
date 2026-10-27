package telemetry

import (
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"testing"
)

func TestMetricResourceFingerprintPreservesTypesAndCollisions(t *testing.T) {
	labels := map[string]string{}
	a := []*commonpb.KeyValue{{Key: "service.name", Value: stringValue("api")}, {Key: "custom.key", Value: stringValue("a")}, {Key: "custom_key", Value: stringValue("b")}}
	addMetricResourceLabels(labels, a)
	if _, ok := labels["resource_custom_key"]; ok {
		t.Fatal("ambiguous selector exposed")
	}
	first := labels["argus_resource_fingerprint"]
	addMetricResourceLabels(labels, append(a, &commonpb.KeyValue{Key: "argus.source.revision", Value: stringValue("99")}))
	if first != labels["argus_resource_fingerprint"] {
		t.Fatal("transport revision changed logical identity")
	}
	stringIdentity := attributeFingerprint([]*commonpb.KeyValue{{Key: "value", Value: stringValue("true")}}, true)
	boolIdentity := attributeFingerprint([]*commonpb.KeyValue{{Key: "value", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}}}, true)
	if stringIdentity == boolIdentity {
		t.Fatal("resource value types collapsed")
	}
}
