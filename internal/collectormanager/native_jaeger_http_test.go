package collectormanager_test

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/google/uuid"
	jaeger "github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
)

func TestNativeJaegerThriftHTTPAuthenticationDiscoveryAndRotation(t *testing.T) {
	f := newNativeSDKProtocolPipeline(t, "jaeger")
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: f.sdkTLS()}, Timeout: 10 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	tag := func(key, value string) *jaeger.Tag {
		return &jaeger.Tag{Key: key, VType: jaeger.TagType_STRING, VStr: &value}
	}
	send := func(app sdkPipelineApplication, operation string, want int) {
		t.Helper()
		batch := &jaeger.Batch{Process: &jaeger.Process{ServiceName: "api", Tags: []*jaeger.Tag{tag("service.namespace", "orders"), tag("service.instance.id", "replica-one")}}, Spans: []*jaeger.Span{{TraceIdHigh: 1, TraceIdLow: time.Now().UnixNano(), SpanId: 3, OperationName: operation, Flags: 1, StartTime: time.Now().UnixMicro(), Duration: 1000, Tags: []*jaeger.Tag{tag("span.kind", "server")}}}}
		raw, err := thrift.NewTSerializer().Write(f.ctx, batch)
		if err != nil {
			t.Fatal(err)
		}
		call, err := http.NewRequestWithContext(f.ctx, http.MethodPost, "https://"+f.d.port(f.container, "24268")+"/api/traces", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		call.Header.Set("Content-Type", "application/x-thrift")
		if app.token != "" {
			call.Header.Set("Authorization", "Bearer "+app.token)
		}
		response, err := client.Do(call)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("native Thrift HTTP response%d expected%d", response.StatusCode, want)
		}
	}
	for _, app := range f.apps {
		send(app, "thrift-initial", http.StatusAccepted)
	}
	f.waitRows(1, 0, 1, 0)
	invalid := f.apps[0]
	invalid.token = ""
	send(invalid, "must-not-arrive", http.StatusUnauthorized)
	invalid.token = "argus_app." + invalid.key.String() + "." + strings.Repeat("x", 43)
	send(invalid, "must-not-arrive", http.StatusUnauthorized)
	f.exec("UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1 WHERE id=$1", f.apps[0].key)
	send(f.apps[0], "must-not-arrive", http.StatusUnauthorized)
	rotated := f.apps[0]
	rotated.key = uuid.New()
	rotated.token = sdkPipelineToken(t, rotated.key)
	digest := sha256.Sum256([]byte(rotated.token))
	f.exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) SELECT $1,enterprise_id,endpoint_id,application_id,'owned-thrift-rotation',$2,now()+interval '1 hour',created_by FROM monitoring_ingest_keys WHERE id=$3", rotated.key, digest[:], f.apps[0].key)
	send(rotated, "thrift-rotated", http.StatusAccepted)
	f.waitRows(2, 0, 1, 0)
	for _, app := range f.apps {
		var services int
		if err := f.store.Pool.QueryRow(f.ctx, "SELECT count(*) FROM monitoring_application_services WHERE application_id=$1 AND service_name='api' AND service_namespace='orders'", app.app).Scan(&services); err != nil || services != 1 {
			t.Fatal("native Thrift service identity missing/duplicated", err)
		}
	}
	logs, _ := f.d.command("logs", f.container)
	for _, token := range []string{f.apps[0].token, f.apps[1].token, rotated.token, invalid.token} {
		if strings.Contains(logs, token) {
			t.Fatal("native Thrift authentication leaked a Key")
		}
	}
	t.Log("actual native Jaeger Thrift HTTP, authentication, service discovery, application isolation, revocation and rotation verified")
}
