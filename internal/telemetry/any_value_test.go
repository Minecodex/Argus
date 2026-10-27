package telemetry

import (
	"encoding/json"
	"strings"
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func TestOTLPCompositeValuesPreserveNestedObjectsArraysAndIntegerPrecision(t *testing.T) {
	integer := &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 9223372036854775806}}
	object := &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{{Key: "count", Value: integer}, {Key: "message", Value: stringValue("event")}, {Key: "items", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{integer, stringValue("child"), {Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}}}}}}}}}}
	encoded := anyValueString(object)
	if strings.Contains(encoded, "KvlistValue") || strings.Contains(encoded, "StringValue") {
		t.Fatal("Go protobuf implementation details entered stored values")
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["count"] != json.Number("9223372036854775806") || decoded["message"] != "event" {
		t.Fatal("object meaning changed", decoded)
	}
	array, ok := decoded["items"].([]any)
	if !ok || len(array) != 3 || array[0] != json.Number("9223372036854775806") || array[1] != "child" || array[2] != true {
		t.Fatal("array meaning changed", decoded)
	}
}

func TestOTLPCompositeValidationRejectsAmbiguousAndExcessivelyDeepValues(t *testing.T) {
	duplicate := &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{{Key: "message", Value: stringValue("one")}, {Key: "message", Value: stringValue("two")}}}}}
	if anyValueSize(duplicate) <= maxLogBody {
		t.Fatal("ambiguous object accepted")
	}
	deep := stringValue("leaf")
	for i := 0; i < 34; i++ {
		deep = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{deep}}}}
	}
	if anyValueSize(deep) <= maxLogBody {
		t.Fatal("unbounded nested value accepted")
	}
	if got := anyValueString(integerValue(9223372036854775806)); got != "9223372036854775806" {
		t.Fatal("scalar format changed", got)
	}
}

func integerValue(value int64) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}
}
