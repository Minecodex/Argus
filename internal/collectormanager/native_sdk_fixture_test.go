package collectormanager_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	redisstore "github.com/kakj-go/Argus/internal/storage/redis"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/twmb/franz-go/pkg/kgo"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type sdkPipelineApplication struct {
	app, endpoint, key         uuid.UUID
	token, environment, source string
}

type nativeSDKPipeline struct {
	ingestQuotaHTTPAddress                  string
	quotaRedis                              *redisstore.Client
	ingestQuotaAddress                      string
	t                                       *testing.T
	ctx                                     context.Context
	d                                       *pipelineDocker
	store                                   *postgres.Store
	conn                                    driver.Conn
	router                                  telemetry.TenantTableRouter
	enterprise, host, collector, generation uuid.UUID
	certs                                   pipelineCertificates
	apps                                    []sdkPipelineApplication
	address, container                      string
	private                                 string
	probe                                   *kgo.Client
}

func newNativeSDKPipeline(t *testing.T) *nativeSDKPipeline {
	return newNativeSDKProtocolPipeline(t, "otlp")
}

type nativeSDKFaultOptions struct {
	MetricObserver               func(*metricsrpc.ExportMetricsServiceRequest)
	Duration                     time.Duration
	Migrations                   string
	ExportFailure, ReportFailure *atomic.Bool
	QueueFilesystemBytes         int64
}

func newNativeSDKProtocolPipeline(t *testing.T, protocol string) *nativeSDKPipeline {
	return newNativeSDKProtocolPipelineWithFaults(t, protocol, nativeSDKFaultOptions{})
}
func newNativeSDKProtocolPipelineWithFaults(t *testing.T, protocol string, faults nativeSDKFaultOptions) *nativeSDKPipeline {
	if protocol != "otlp" && protocol != "jaeger" {
		t.Fatal("unsupported SDK fixture protocol")
	}
	signals := []string{"traces", "logs", "metrics"}
	port := "24318"
	if protocol == "jaeger" {
		signals = []string{"traces"}
		port = "24250"
	}
	t.Helper()
	if os.Getenv("ARGUS_MONITORING_NATIVE_PIPELINE") != "1" {
		t.Skip("requires explicitly owned native pipeline dependencies")
	}
	f := &nativeSDKPipeline{t: t, d: newPipelineDocker(t), enterprise: uuid.New(), host: uuid.New(), collector: uuid.New(), generation: uuid.New()}
	duration := 4 * time.Minute
	if faults.Duration > 0 {
		duration = faults.Duration
	}
	ctx, cancel := context.WithTimeout(t.Context(), duration)
	t.Cleanup(cancel)
	f.ctx = ctx
	database := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if database == "" || os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS") == "" {
		t.Fatal("explicit disposable backend endpoints required")
	}
	migrations := faults.Migrations
	if migrations == "" {
		migrations = filepath.Join("..", "..", "migrations", "postgresql")
	}
	if err := postgres.RunMigrations(ctx, database, migrations, postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	var err error
	f.store, err = postgres.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.store.Close)
	f.conn, err = telemetry.OpenClickHouse(os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS"), "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.conn.Close() })
	distribution, creator := uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned real SDK pipeline',$2,'UTC')", f.enterprise, f.enterprise.String())
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Controlled SDK transport','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", f.host, f.enterprise)
	f.exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,components,artifact_manifest,catalog_revision) VALUES($1,$2,$3,$4,$5,'supported',$6,'[]',1)", distribution, distribution.String(), configbundle.DistributionVersion, configbundle.CollectorVersion, configbundle.CatalogConfigSchemaVersion, configbundle.DistributionComponents("linux_amd64"))
	f.exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", f.collector, f.enterprise, f.host, distribution, f.generation)
	endpoints := []configbundle.ApplicationEndpoint{}
	for _, environment := range []string{"production", "testing"} {
		app := sdkPipelineApplication{app: uuid.New(), endpoint: uuid.New(), key: uuid.New(), environment: environment}
		app.token = sdkPipelineToken(t, app.key)
		digest := sha256.Sum256([]byte(app.token))
		f.exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Real SDK application',$4,$5)", app.app, f.enterprise, app.app.String(), environment, creator)
		f.exec("INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,collector_id,created_by) VALUES($1,$2,$3,'Native SDK',$6,$7,$4,$5)", app.endpoint, f.enterprise, app.app, f.collector, creator, protocol, signals)
		f.exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) VALUES($1,$2,$3,$4,'owned-test',$5,now()+interval '1 hour',$6)", app.key, f.enterprise, app.endpoint, app.app, digest[:], creator)
		endpoints = append(endpoints, configbundle.ApplicationEndpoint{ID: app.endpoint.String(), ApplicationID: app.app.String(), Protocol: protocol, Signals: signals, Version: 1})
		f.apps = append(f.apps, app)
	}
	f.certs = pipelineTLS(t, f.d, f.collector)
	f.exec("INSERT INTO telemetry_certificates(id,collector_id,serial_number,uri_san,csr_hash,certificate_hash,certificate_request_name,issuer_generation,not_before,not_after,certificate_usage) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$9,'clientAuth')", uuid.New(), f.collector, f.certs.serial, f.certs.uri, f.certs.csrHash[:], f.certs.certificateHash[:], f.d.owner, f.certs.notBefore, f.certs.notAfter)
	f.exec("INSERT INTO pki_certificate_identities(serial_number,subject_kind,subject_id,enterprise_id,uri_san,extended_key_usage,issuer_generation,certificate_sha256,not_before,not_after) VALUES($1,'collector',$2,$3,$4,'clientAuth',1,$5,$6,$7)", f.certs.serial, f.collector.String(), f.enterprise, f.certs.uri, hex.EncodeToString(f.certs.certificateHash[:]), f.certs.notBefore, f.certs.notAfter)
	manager := telemetry.ClickHouseTenantSchemaManager{Conn: f.conn, Router: f.router}
	if err = manager.EnsureTenant(ctx, f.enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), f.enterprise); err != nil {
			t.Error(err)
		}
	})
	f.exec("INSERT INTO enterprise_telemetry_tables(enterprise_id,schema_version,status,ready_at) VALUES($1,$2,'ready',now())", f.enterprise, telemetry.TelemetrySchemaVersion)
	producer, consumer, probe := f.d.kafka()
	f.probe = probe
	redisClient := f.d.redis()
	f.quotaRedis = redisClient
	writer := &telemetry.Writer{Kafka: consumer, ClickHouse: f.conn, Router: f.router, Control: telemetry.PostgresWriterControl{Queries: f.store.Queries}}
	writerCtx, stopWriter := context.WithCancel(ctx)
	writerDone := make(chan error, 1)
	go func() { writerDone <- writer.Run(writerCtx) }()
	t.Cleanup(func() {
		stopWriter()
		select {
		case err := <-writerDone:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("SDK pipeline writer did not stop")
		}
	})
	ingest := &telemetry.IngestServer{Control: telemetry.PostgresIngestControl{Queries: f.store.Queries}, Kafka: producer, Redis: redisClient, Applications: monitoring.Service{Store: f.store}, DeliveryReports: telemetry.CollectorDeliveryService{Store: f.store}}
	serverTLS := &tls.Config{Certificates: []tls.Certificate{f.certs.server}, ClientCAs: f.certs.pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	f.ingestQuotaAddress = fmt.Sprintf("127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if value, ok := request.(*metricsrpc.ExportMetricsServiceRequest); ok && faults.MetricObserver != nil {
			faults.MetricObserver(value)
		}
		if faults.ExportFailure != nil && faults.ExportFailure.Load() && strings.HasPrefix(info.FullMethod, "/opentelemetry.proto.collector.") {
			return nil, status.Error(codes.Unavailable, "owned native export outage")
		}
		return handler(ctx, request)
	}))
	ingest.RegisterGRPC(server)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	httpListener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := ingest.HTTPHandler()
	authority := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Certificate enrollment/rotation is a controlled fixture. SDK Key
		// authentication, source admission and service discovery are real.
		if r.URL.Path == "/v1/identity/trust-bundle" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if faults.ReportFailure != nil && faults.ReportFailure.Load() && r.URL.Path == "/v1/collector-delivery" {
			http.Error(w, "owned report outage", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}), TLSConfig: serverTLS.Clone(), ReadHeaderTimeout: 5 * time.Second}
	go authority.Serve(tls.NewListener(httpListener, authority.TLSConfig))
	t.Cleanup(func() { _ = authority.Close() })
	f.ingestQuotaHTTPAddress = fmt.Sprintf("https://localhost:%d", httpListener.Addr().(*net.TCPAddr).Port)
	authorityAddress := fmt.Sprintf("https://host.docker.internal:%d", httpListener.Addr().(*net.TCPAddr).Port)
	bundle, err := configbundle.Render(configbundle.RenderInput{CollectorID: f.collector.String(), ResourceID: f.host.String(), ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: f.generation.String(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: authorityAddress + "/v1/identity/enroll", IngestHTTPEndpoint: authorityAddress, IngestGRPCEndpoint: fmt.Sprintf("grpcs://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), ApplicationEndpoints: endpoints})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,decode(repeat('00',32),'hex'),'effective')", uuid.New(), f.collector, bundle)
	sources, err := configbundle.Sources(bundle)
	if err != nil || len(sources) != len(f.apps) {
		t.Fatalf("SDK source manifest: %v", err)
	}
	for _, source := range sources {
		for i := range f.apps {
			if f.apps[i].endpoint.String() == source.IngestEndpointID {
				f.apps[i].source = source.ID
			}
		}
		f.exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,ingest_endpoint_id,ingest_endpoint_version) VALUES($1,$2,$3,$4,'application',$5,$6,$8,$9,1,decode(repeat('00',32),'hex'),'application/v1',$7,1)", uuid.MustParse(source.ID), f.enterprise, f.collector, f.generation, uuid.MustParse(source.ResourceID), source.Key, uuid.MustParse(source.IngestEndpointID), protocol, signals)
	}
	configRaw, err := configbundle.Extract(bundle, "host")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if json.Unmarshal(configRaw, &config) != nil {
		t.Fatal("SDK configuration")
	}
	identity := config["extensions"].(map[string]any)["argus_identity"].(map[string]any)
	identity["rotate_before"] = "10m" // Fixture certificates expire after one hour; check interval remains five minutes.
	configPath := f.d.file("collector.json", mustSDKJSON(t, config))
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	containerArgs := []string{"--memory=512m", "--cpus=1", "--cap-drop=ALL", "-p", "127.0.0.1::" + port, "-p", "127.0.0.1::24317", "-p", "127.0.0.1::24268", "--mount", "type=bind,source=" + binary + ",target=/argus,readonly", "--mount", "type=bind,source=" + configPath + ",target=/config.json,readonly", "--mount", "type=bind,source=" + f.d.directory + ",target=/var/lib/argus-otelcol/identity"}
	if faults.QueueFilesystemBytes != 0 {
		if faults.QueueFilesystemBytes < 4<<20 || faults.QueueFilesystemBytes > 128<<20 {
			t.Fatal("invalid owned queue filesystem fault size")
		}
		containerArgs = append(containerArgs, "--tmpfs", fmt.Sprintf("/var/lib/argus-otelcol/identity/queue:rw,size=%d,mode=0700", faults.QueueFilesystemBytes))
	}
	containerArgs = append(containerArgs, "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json")
	f.container = f.d.start("sdk-collector", containerArgs...)
	f.address = f.d.port(f.container, port)
	deadline := time.Now().Add(15 * time.Second)
	for {
		connection, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", f.address, f.sdkTLS())
		if e == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SDK Collector TLS listener did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return f
}

func (f *nativeSDKPipeline) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(f.ctx, query, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *nativeSDKPipeline) sdkTLS() *tls.Config {
	return &tls.Config{RootCAs: f.certs.pool, ServerName: "collector-" + f.collector.String() + ".argus.telemetry", MinVersion: tls.VersionTLS13}
}
func mustSDKJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sdkPipelineToken(t *testing.T, id uuid.UUID) string {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	defer clear(secret)
	return "argus_app." + id.String() + "." + base64.RawURLEncoding.EncodeToString(secret)
}
