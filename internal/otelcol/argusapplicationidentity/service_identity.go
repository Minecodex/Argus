package argusapplicationidentity

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func validateServiceIdentity(attrs pcommon.Map) error {
	rejected := func() error {
		return consumererror.NewPermanent(status.Error(codes.InvalidArgument, "application service identity rejected"))
	}
	name, exists := attrs.Get("service.name")
	if !exists || name.Type() != pcommon.ValueTypeStr || !validSDKName(name.Str(), 200) || strings.HasPrefix(strings.ToLower(name.Str()), "unknown_service") {
		return rejected()
	}
	for key, limit := range map[string]int{"service.namespace": 200, "service.instance.id": 512, "telemetry.sdk.language": 100} {
		value, exists := attrs.Get(key)
		if exists && (value.Type() != pcommon.ValueTypeStr || value.Str() != "" && !validSDKName(value.Str(), limit)) {
			return rejected()
		}
	}
	return nil
}

func validSDKName(value string, limit int) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}
