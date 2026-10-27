package telemetry

import (
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"strings"
	"unicode/utf8"
)

// Shared credential projection runs on OTLP values without converting numbers
// through JSON, preserving integer precision and signal identities.
func redactOTLPAttributes(values []*commonpb.KeyValue) error {
	credentialPair := false
	for _, item := range values {
		if item != nil && (item.Key == "key" || item.Key == "name") {
			for _, part := range strings.FieldsFunc(item.Value.GetStringValue(), func(r rune) bool { return r == '_' || r == '-' || r == '.' }) {
				credentialPair = credentialPair || datapolicy.CredentialKey(part)
			}
		}
	}
	for _, item := range values {
		if item == nil {
			continue
		}
		if datapolicy.CredentialKey(item.Key) || credentialPair && (item.Key == "value" || item.Key == "values") {
			item.Value = stringValue(datapolicy.Marker)
			continue
		}
		if err := redactOTLPValue(item.Value); err != nil {
			return err
		}
	}
	return nil
}
func redactOTLPValue(value *commonpb.AnyValue) error {
	if value == nil {
		return nil
	}
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		clean, _, err := datapolicy.Project(v.StringValue)
		if err != nil {
			return err
		}
		text, ok := clean.(string)
		if !ok {
			return errIngestRejected
		}
		v.StringValue = text
	case *commonpb.AnyValue_BytesValue:
		if utf8.Valid(v.BytesValue) {
			clean, _, err := datapolicy.Project(string(v.BytesValue))
			if err != nil {
				return err
			}
			text, ok := clean.(string)
			if !ok {
				return errIngestRejected
			}
			clear(v.BytesValue)
			v.BytesValue = []byte(text)
		}
	case *commonpb.AnyValue_KvlistValue:
		return redactOTLPAttributes(v.KvlistValue.GetValues())
	case *commonpb.AnyValue_ArrayValue:
		for _, child := range v.ArrayValue.GetValues() {
			if err := redactOTLPValue(child); err != nil {
				return err
			}
		}
	}
	return nil
}
