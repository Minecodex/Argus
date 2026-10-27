package argusapplicationidentity

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestServiceIdentityRejectsMissingDefaultAndMalformedNames(t *testing.T) {
	for _, name := range []string{"", "unknown_service:binary", "UNKNOWN_SERVICE", " orders", "orders\n", strings.Repeat("x", 201)} {
		attrs := pcommon.NewMap()
		if name != "" {
			attrs.PutStr("service.name", name)
		}
		if err := validateServiceIdentity(attrs); err == nil || !consumererror.IsPermanent(err) {
			t.Errorf("invalid service name accepted: length%d", len(name))
		}
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("service.name", "订单 api")
	attrs.PutStr("service.namespace", "payments")
	if err := validateServiceIdentity(attrs); err != nil {
		t.Fatal("valid service without instance was rejected", err)
	}
	attrs.PutInt("service.instance.id", 1)
	if validateServiceIdentity(attrs) == nil {
		t.Fatal("non-string instance accepted")
	}
	attrs.PutStr("service.instance.id", strings.Repeat("x", 513))
	if validateServiceIdentity(attrs) == nil {
		t.Fatal("oversized instance accepted")
	}
}
