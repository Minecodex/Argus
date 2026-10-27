package argusapplicationidentity

import (
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

type testAuth map[string]string

func (a testAuth) GetAttribute(key string) any { return a[key] }
func (a testAuth) GetAttributeNames() []string {
	return []string{"key_id", "endpoint_id", "endpoint_version", "signals"}
}
func TestApplicationBindingsAreFixedBeforeBatching(t *testing.T) {
	endpoint, source, key := uuid.NewString(), uuid.NewString(), uuid.NewString()
	config := Config{Bindings: []Binding{{EndpointID: endpoint, EndpointVersion: 2, SourceID: source, SourceRevision: 4}}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	auth := testAuth{"key_id": key, "endpoint_id": endpoint, "endpoint_version": "2", "signals": "traces"}
	ctx := client.NewContext(t.Context(), client.Info{Auth: auth})
	b, value, err := authenticate(ctx, &config, "traces")
	if err != nil {
		t.Fatal(err)
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("argus.resource.id", "forged")
	attrs.PutStr("argus.application.key.id", "forged")
	attrs.PutStr("service.name", "orders")
	stamp(attrs, b, value)
	if _, exists := attrs.Get("argus.resource.id"); exists {
		t.Fatal("SDK ownership claim survived")
	}
	if v, _ := attrs.Get("argus.application.key.id"); v.Str() != key {
		t.Fatal("key scope lost")
	}
	if v, _ := attrs.Get("service.name"); v.Str() != "orders" {
		t.Fatal("service identity lost")
	}
	if _, _, err = authenticate(ctx, &config, "logs"); err == nil {
		t.Fatal("unapproved signal admitted")
	}
	auth["endpoint_version"] = "3"
	if _, _, err = authenticate(ctx, &config, "traces"); err == nil {
		t.Fatal("new endpoint used stale binding")
	}
	auth["endpoint_version"] = "2"
	auth["endpoint_id"] = uuid.NewString()
	if _, _, err = authenticate(ctx, &config, "traces"); err == nil {
		t.Fatal("foreign application bound")
	}
	auth["transient_error"] = "application_auth_unavailable"
	if _, _, err = authenticate(ctx, &config, "traces"); err == nil || consumererror.IsPermanent(err) {
		t.Fatal("authority outage was accepted or made permanent")
	}
}
