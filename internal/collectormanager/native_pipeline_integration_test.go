package collectormanager_test

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// Actual native filelog -> mTLS Ingest -> authenticated Kafka -> Writer ->
// ClickHouse. The authority rows and CA are controlled fixtures, not formal
// host enrollment, deployed Portal, or Agent/model acceptance.
func TestNativeLogPipelineWaitsForApplyAndSurvivesCollectorCrash(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_PIPELINE") != "1" {
		t.Skip("requires disposable PostgreSQL, ClickHouse, Docker and the locked Collector")
	}
	for _, business := range []bool{false, true} {
		name := "system"
		if business {
			name = "business"
		}
		t.Run(name, func(t *testing.T) { runNativeLogPipeline(t, business) })
	}
}

func runNativeLogPipeline(t *testing.T, business bool) {
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		t.Fatal("locked Collector binary required")
	}
	database, address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL"), os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if database == "" || address == "" {
		t.Fatal("explicit owned backend endpoints required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	if err = postgres.RunMigrations(ctx, database, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	d := newPipelineDocker(t)
	enterprise, host, collector, generation, distribution, collection, creator := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned native pipeline',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Controlled native fixture','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, enterprise)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'test',$3,'argus.collector_config/v1','supported','[]',1)", distribution, distribution.String(), configbundle.CollectorVersion)
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','installing',$5,1,0)", collector, enterprise, host, distribution, generation)
	rule := json.RawMessage(`{"scope":"system","include":["/input/pipeline.log"],"start_at":"beginning","format":"json"}`)
	target, targetType := host, "host"
	var applicationArg, serviceArg any
	var service uuid.UUID
	var declared *configbundle.CollectionService
	if business {
		target, targetType = uuid.New(), "application"
		service = uuid.New()
		applicationArg, serviceArg = target, service
		exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Native application','production',$4)", target, enterprise, target.String(), creator)
		exec("INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_name,service_namespace,runtime) VALUES($1,$2,$3,'orders','business','declared')", service, enterprise, target)
		rule = json.RawMessage(`{"scope":"business","include":["/input/pipeline.log"],"start_at":"beginning","format":"json"}`)
		declared = &configbundle.CollectionService{ID: service.String(), Name: "orders", Namespace: "business", Environment: "production"}
	}
	assignment := configbundle.Collection{ID: collection.String(), Integration: "filelog", TargetType: targetType, TargetID: target.String(), Identity: json.RawMessage(`{}`), Config: rule, Enabled: true, Revision: 1, AssignmentEpoch: 1, Service: declared}
	raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collector.String(), ResourceID: host.String(), ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: generation.String(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: []configbundle.Collection{assignment}})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := configbundle.Sources(raw)
	if err != nil || len(sources) != 1 {
		t.Fatalf("native source manifest: %d %v", len(sources), err)
	}
	source := sources[0]
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,decode(repeat('00',32),'hex'),'applying')", uuid.New(), collector, raw)
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,config,application_id,service_id,created_by) VALUES($1,$2,'Owned file source','filelog',$3,$4,$5,$6,$7,$8,$9)", collection, enterprise, targetType, target, collector, rule, applicationArg, serviceArg, creator)
	if _, err = store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: enterprise, CollectionID: collection, ActorID: creator}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id) VALUES($1,$2,1,$3)", collection, enterprise, collector)
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch,application_service_id) VALUES($1,$2,$3,$4,$5,$6,$7,'filelog',ARRAY['logs'],1,decode(repeat('00',32),'hex'),'v1',$8,1,1,$9)", uuid.MustParse(source.ID), enterprise, collector, generation, targetType, target, source.Key, collection, serviceArg)
	certs := pipelineTLS(t, d, collector)
	exec("INSERT INTO telemetry_certificates(id,collector_id,serial_number,uri_san,csr_hash,certificate_hash,certificate_request_name,issuer_generation,not_before,not_after,certificate_usage) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$9,'clientAuth')", uuid.New(), collector, certs.serial, certs.uri, certs.csrHash[:], certs.certificateHash[:], d.owner, certs.notBefore, certs.notAfter)
	exec("INSERT INTO pki_certificate_identities(serial_number,subject_kind,subject_id,enterprise_id,uri_san,extended_key_usage,issuer_generation,certificate_sha256,not_before,not_after) VALUES($1,'collector',$2,$3,$4,'clientAuth',1,$5,$6,$7)", certs.serial, collector.String(), enterprise, certs.uri, hex.EncodeToString(certs.certificateHash[:]), certs.notBefore, certs.notAfter)
	router := telemetry.TenantTableRouter{}
	manager := telemetry.ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err = manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), enterprise); err != nil {
			t.Error(err)
		}
	})
	exec("INSERT INTO enterprise_telemetry_tables(enterprise_id,schema_version,status,ready_at) VALUES($1,$2,'ready',now())", enterprise, telemetry.TelemetrySchemaVersion)
	producer, consumer, probe := d.kafka()
	redisClient := d.redis()
	writer := &telemetry.Writer{Kafka: consumer, ClickHouse: conn, Router: router, Control: telemetry.PostgresWriterControl{Queries: store.Queries}}
	writerCtx, stopWriter := context.WithCancel(ctx)
	writerDone := make(chan struct{})
	var writerError error
	go func() { writerError = writer.Run(writerCtx); close(writerDone) }()
	t.Cleanup(func() {
		stopWriter()
		select {
		case <-writerDone:
			if writerError != nil {
				t.Error(writerError)
			}
		case <-time.After(10 * time.Second):
			t.Error("writer did not stop")
		}
	})
	ingest := &telemetry.IngestServer{Control: telemetry.PostgresIngestControl{Queries: store.Queries}, Kafka: producer, Redis: redisClient, Applications: monitoring.Service{Store: store}}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	var unavailable, accepted atomic.Int64
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{certs.server}, ClientCAs: certs.pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13})), grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		result, err := handler(ctx, request)
		if status.Code(err) == codes.Unavailable {
			unavailable.Add(1)
		}
		if err == nil {
			accepted.Add(1)
		}
		return result, err
	}))
	ingest.RegisterGRPC(server)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	public, err := configbundle.Extract(raw, "host")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if json.Unmarshal(public, &config) != nil {
		t.Fatal("rendered config")
	}
	config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/state/storage", "create_directory": true}}
	config["service"].(map[string]any)["extensions"] = []string{"file_storage"}
	config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]any{"ca_file": "/tls/ca.pem", "cert_file": "/tls/client.pem", "key_file": "/tls/client-key.pem", "server_name_override": "host.docker.internal"}, "timeout": "3s", "sending_queue": map[string]any{"enabled": true, "storage": "file_storage", "queue_size": 1024}, "retry_on_failure": map[string]any{"enabled": true, "initial_interval": "1s", "max_interval": "2s", "max_elapsed_time": "5m"}}}
	configRaw, _ := json.Marshal(config)
	configPath := d.file("collector.json", configRaw)
	inputDir, stateDir := filepath.Join(d.directory, "input"), filepath.Join(d.directory, "state")
	for _, path := range []string{inputDir, stateDir} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	inputPath := filepath.Join(inputDir, "pipeline.log")
	const private = "pipeline-private-canary-never-in-kafka"
	appendLines := func(from, to int) {
		t.Helper()
		file, err := os.OpenFile(inputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		for i := from; i < to; i++ {
			line, _ := json.Marshal(map[string]string{"message": fmt.Sprintf("pipeline-event-%02d", i), "password": private})
			if _, err = file.Write(append(line, '\n')); err != nil {
				t.Fatal(err)
			}
		}
		if err = file.Sync(); err != nil {
			t.Fatal(err)
		}
	}
	appendLines(0, 10)
	run := func(name string) string {
		return d.start(name, "--memory=512m", "--cpus=1", "--read-only", "--cap-drop=ALL", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+configPath+",target=/config.json,readonly", "--mount", "type=bind,source="+d.directory+",target=/tls,readonly", "--mount", "type=bind,source="+inputDir+",target=/input,readonly", "--mount", "type=bind,source="+stateDir+",target=/state", "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json")
	}
	first := run("collector-first")
	deadline := time.Now().Add(30 * time.Second)
	for unavailable.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if unavailable.Load() == 0 {
		logs, _ := d.command("logs", "--tail", "10", first)
		t.Fatalf("native pipeline did not reach retryable admission: %s", strings.ReplaceAll(logs, private, "[REDACTED]"))
	}
	if accepted.Load() != 0 {
		t.Fatal("unapplied source acknowledged data")
	}
	time.Sleep(2 * time.Second) // Native checkpoint flush, with records in the durable exporter queue.
	d.must("kill", "--signal=KILL", first)
	exec("UPDATE collector_instances SET effective_revision=1,status='converged' WHERE id=$1", collector)
	exec("UPDATE collector_config_revisions SET status='effective' WHERE collector_id=$1 AND revision=1", collector)
	// Controlled registration must activate the same responsibility tuple that
	// production apply outcomes establish; a revision alone cannot admit data.
	exec("UPDATE monitoring_collections SET effective_revision=1,effective_collector_id=$2,effective_assignment_epoch=1,apply_status='effective' WHERE id=$1", collection, collector)
	exec("UPDATE monitoring_collection_assignments SET activated_at=now() WHERE collection_id=$1 AND epoch=1 AND collector_id=$2", collection, collector)
	appendLines(10, 12)
	second := run("collector-second")
	var rows []map[string]any
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		result, queryErr := kql.Execute(ctx, conn, router, kql.Request{Expression: "*", Start: time.Now().Add(-5 * time.Minute), End: time.Now().Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.ID + ":1"}}, Budget: kql.Budget{MaxRows: 100, MaxScanBytes: 64 << 20, Timeout: 5 * time.Second}})
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		rows = result.Data
		if len(rows) >= 12 {
			break
		}
		select {
		case <-writerDone:
			t.Fatalf("writer stopped: %v", writerError)
		default:
		}
		time.Sleep(500 * time.Millisecond)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		body, _ := row["body"].(string)
		if strings.Contains(body, private) {
			t.Fatal("private value reached stored logs")
		}
		if business && row["service_name"] != "orders" {
			t.Fatalf("native business service attribution lost: %v", row["service_name"])
		}
		for i := 0; i < 12; i++ {
			value := fmt.Sprintf("pipeline-event-%02d", i)
			if strings.Contains(body, value) {
				seen[value] = true
			}
		}
	}
	if len(seen) != 12 || len(rows) != 12 {
		t.Fatalf("durable native pipeline coverage: unique=%d stored=%d unavailable=%d accepted=%d", len(seen), len(rows), unavailable.Load(), accepted.Load())
	}
	probeCtx, stopProbe := context.WithTimeout(ctx, 10*time.Second)
	defer stopProbe()
	fetches := probe.PollFetches(probeCtx)
	if err := fetches.Err(); err != nil {
		t.Fatal(err)
	}
	if len(fetches.Records()) == 0 {
		t.Fatal("no actual Kafka records available for privacy validation")
	}
	for _, record := range fetches.Records() {
		if strings.Contains(string(record.Value), private) {
			t.Fatal("raw private value reached Kafka before redaction")
		}
	}
	for _, container := range []string{first, second} {
		logs, _ := d.command("logs", container)
		if strings.Contains(logs, private) {
			t.Fatal("native diagnostics exposed a private record")
		}
	}
	var sampleSeen bool
	if err = store.Pool.QueryRow(ctx, "SELECT last_sample_at IS NOT NULL FROM monitoring_collections WHERE id=$1", collection).Scan(&sampleSeen); err != nil || !sampleSeen {
		t.Fatalf("accepted samples did not update actual collection state: %v", err)
	}
	if business {
		wrong, err := kql.Execute(ctx, conn, router, kql.Request{Expression: "*", Start: time.Now().Add(-5 * time.Minute), End: time.Now().Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.ID + ":1"}}, Budget: kql.Budget{MaxRows: 100, MaxScanBytes: 64 << 20, Timeout: 5 * time.Second}})
		if err != nil || len(wrong.Data) != 0 {
			t.Fatalf("application logs leaked into executor host scope: %v", err)
		}
		var observed bool
		if err = store.Pool.QueryRow(ctx, "SELECT first_seen_at IS NOT NULL AND last_seen_at IS NOT NULL FROM monitoring_application_services WHERE id=$1", service).Scan(&observed); err != nil || !observed {
			t.Fatal("actual business logs did not observe the declared service")
		}
	}
	t.Logf("scope=controlled_registration_real_native_mtls_ingest_kafka_writer_clickhouse resource_type=%s records=%d retryable_before_apply=%d accepted=%d owner=%s", targetType, len(rows), unavailable.Load(), accepted.Load(), d.owner)
}
