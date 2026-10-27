package telemetry

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// Convert OTLP's union to semantic values, never its generated Go wrappers.
// Native int64 values reach json.Marshal without a float64 round trip.
func otlpJSONValue(value *commonpb.AnyValue, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("OTLP value nesting exceeds the supported depth")
	}
	if value == nil || value.Value == nil {
		return nil, nil
	}
	switch item := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return item.StringValue, nil
	case *commonpb.AnyValue_BoolValue:
		return item.BoolValue, nil
	case *commonpb.AnyValue_IntValue:
		return item.IntValue, nil
	case *commonpb.AnyValue_DoubleValue:
		if math.IsInf(item.DoubleValue, 0) || math.IsNaN(item.DoubleValue) {
			return strconv.FormatFloat(item.DoubleValue, 'g', -1, 64), nil
		}
		return item.DoubleValue, nil
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(item.BytesValue), nil
	case *commonpb.AnyValue_ArrayValue:
		result := make([]any, 0, len(item.ArrayValue.GetValues()))
		for _, child := range item.ArrayValue.GetValues() {
			decoded, err := otlpJSONValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, decoded)
		}
		return result, nil
	case *commonpb.AnyValue_KvlistValue:
		result := make(map[string]any, len(item.KvlistValue.GetValues()))
		for _, entry := range item.KvlistValue.GetValues() {
			if entry == nil {
				return nil, errors.New("OTLP object has an invalid member")
			}
			if _, exists := result[entry.Key]; exists {
				return nil, errors.New("OTLP object has a duplicate key")
			}
			decoded, err := otlpJSONValue(entry.Value, depth+1)
			if err != nil {
				return nil, err
			}
			result[entry.Key] = decoded
		}
		return result, nil
	default:
		return nil, errors.New("unsupported OTLP value")
	}
}

func otlpCompositeJSON(value *commonpb.AnyValue) ([]byte, error) {
	decoded, err := otlpJSONValue(value, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}
