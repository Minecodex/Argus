package collectormanager_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	jaeger "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Collector, authentication/discovery, Kafka, Writer and ClickHouse are real.
// Enrollment and the control registration remain explicit fixtures.
func TestNativeSDKGRPCProtocolsDiscoverServicesAndFenceRevokedKeys(t *testing.T) {
	for _, protocol := range []string{"otlp", "jaeger"} {
		t.Run(protocol, func(t *testing.T) {
			f := newNativeSDKProtocolPipeline(t, protocol)
			send := func(app sdkPipelineApplication, operation string) error {
				if protocol == "jaeger" {
					connection, err := grpc.NewClient(f.address, grpc.WithTransportCredentials(credentials.NewTLS(f.sdkTLS())))
					if err != nil {
						return err
					}
					defer connection.Close()
					_, err = jaeger.NewCollectorServiceClient(connection).PostSpans(metadata.NewOutgoingContext(f.ctx, metadata.Pairs("authorization", "Bearer "+app.token)), &jaeger.PostSpansRequest{Batch: model.Batch{
						Process: &model.Process{ServiceName: "api", Tags: []model.KeyValue{
							{Key: "service.namespace", VType: model.ValueType_STRING, VStr: "orders"},
							{Key: "service.instance.id", VType: model.ValueType_STRING, VStr: "replica-one"},
							{Key: "argus.application.id", VType: model.ValueType_STRING, VStr: f.apps[1].app.String()},
						}}, Spans: []*model.Span{{TraceID: model.TraceID{High: 1, Low: uint64(time.Now().UnixNano())}, SpanID: 3, OperationName: operation, StartTime: time.Now(), Duration: time.Millisecond, Tags: []model.KeyValue{{Key: "span.kind", VType: model.ValueType_STRING, VStr: "server"}}}},
					}})
					return err
				}
				exporter, err := otlptracegrpc.New(f.ctx, otlptracegrpc.WithEndpoint(f.d.port(f.container, "24317")), otlptracegrpc.WithTLSCredentials(credentials.NewTLS(f.sdkTLS())), otlptracegrpc.WithHeaders(map[string]string{"Authorization": "Bearer " + app.token}), otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{Enabled: false}))
				if err != nil {
					return err
				}
				capture := &sdkTraceExport{SpanExporter: exporter}
				provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(capture), sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "api"), attribute.String("service.namespace", "orders"), attribute.String("service.instance.id", "replica-one"), attribute.String("argus.application.id", f.apps[1].app.String()))))
				_, span := provider.Tracer("plan8.actual-go-grpc-sdk").Start(context.Background(), operation)
				span.End()
				shutdown := provider.Shutdown(f.ctx)
				if capture.failure() != nil {
					return capture.failure()
				}
				return shutdown
			}
			for _, app := range f.apps {
				if err := send(app, "initial-request"); err != nil {
					t.Fatal("native protocol export failed", err)
				}
			}
			f.waitRows(1, 0, 1, 0)
			tables, err := f.router.Tables(f.enterprise)
			if err != nil {
				t.Fatal(err)
			}
			services := map[uuid.UUID]bool{}
			for _, app := range f.apps {
				var service uuid.UUID
				if err = f.store.Pool.QueryRow(f.ctx, "SELECT id FROM monitoring_application_services WHERE application_id=$1 AND service_name='api' AND service_namespace='orders'", app.app).Scan(&service); err != nil {
					t.Fatal("native service identity missing", err)
				}
				if services[service] {
					t.Fatal("protocol merged services across applications")
				}
				services[service] = true
				var source, environment, serviceID, sourceType string
				if err = f.conn.QueryRow(f.ctx, "SELECT toString(source_id),resource_attributes['deployment.environment.name'],resource_attributes['argus.service.id'],source_type FROM "+tables.Traces+" WHERE resource_id=? LIMIT 1", app.app).Scan(&source, &environment, &serviceID, &sourceType); err != nil {
					t.Fatal(err)
				}
				if source != app.source || environment != app.environment || serviceID != service.String() || sourceType != protocol {
					t.Fatal("protocol lost trusted source, application environment, or stable service")
				}
			}
			invalid := f.apps[0]
			invalid.token = "argus_app." + invalid.key.String() + "." + strings.Repeat("x", 43)
			if err = send(invalid, "forged-key"); status.Code(err) != codes.Unauthenticated {
				t.Fatal("forged key was not rejected by native authentication", status.Code(err))
			}
			f.exec("UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1 WHERE id=$1", f.apps[0].key)
			if err = send(f.apps[0], "revoked-request"); status.Code(err) != codes.Unauthenticated {
				t.Fatal("revoked key remained usable", status.Code(err))
			}
			rotated := f.apps[0]
			rotated.key = uuid.New()
			rotated.token = sdkPipelineToken(t, rotated.key)
			digest := sha256.Sum256([]byte(rotated.token))
			f.exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) SELECT $1,enterprise_id,endpoint_id,application_id,'owned-grpc-rotation',$2,now()+interval '1 hour',created_by FROM monitoring_ingest_keys WHERE id=$3", rotated.key, digest[:], f.apps[0].key)
			if err = send(rotated, "rotated-request"); err != nil {
				t.Fatal("rotated native key failed", err)
			}
			f.waitRows(2, 0, 1, 0)
			var count int
			if err = f.store.Pool.QueryRow(f.ctx, "SELECT count(*) FROM monitoring_application_services WHERE application_id=$1", rotated.app).Scan(&count); err != nil || count != 1 {
				t.Fatal("rotation duplicated the stable service", err)
			}
			diagnostics, _ := f.d.command("logs", f.container)
			for _, token := range []string{f.apps[0].token, f.apps[1].token, rotated.token, invalid.token} {
				if strings.Contains(diagnostics, token) {
					t.Fatal("native protocol authentication leaked a Key")
				}
			}
			t.Log("actual native protocol, application discovery, source/environment isolation, Key rejection/rotation and persisted query verified")
		})
	}
}
