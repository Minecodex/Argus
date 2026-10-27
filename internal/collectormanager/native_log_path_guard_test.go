package collectormanager_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type pathGuardSink struct {
	logsrpc.UnimplementedLogsServiceServer
	mu     sync.Mutex
	token  string
	bodies []string
	paths  []string
	paused bool
}

func (s *pathGuardSink) Export(ctx context.Context, in *logsrpc.ExportLogsServiceRequest) (*logsrpc.ExportLogsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("x-plan8-path-guard"); len(values) != 1 || values[0] != s.token {
		return nil, status.Error(codes.Unauthenticated, "test sink authentication")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused {
		return nil, status.Error(codes.Unavailable, "controlled sink outage")
	}
	for _, resource := range in.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			for _, record := range scope.LogRecords {
				body := record.Body.GetStringValue()
				if body == "" {
					raw, _ := protojson.Marshal(record.Body)
					body = string(raw)
				}
				s.bodies = append(s.bodies, body)
				for _, attribute := range record.Attributes {
					if attribute.Key == "log.file.path_resolved" {
						s.paths = append(s.paths, attribute.Value.GetStringValue())
					}
				}
			}
		}
	}
	return &logsrpc.ExportLogsServiceResponse{}, nil
}
func (s *pathGuardSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.bodies...)
}

func (s *pathGuardSink) pause(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = value
}

func TestNativeFilelogRejectsReservedRuntimeSymlinkBeforeParsing(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_LOGS") != "1" {
		t.Skip("requires Docker and locked Linux Collector")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	d := newPipelineDocker(t)
	safe := d.file("safe.log", []byte("{\"message\":\"safe-file-marker\",\"log.file.path_resolved\":\"/fake/untrusted.log\"}\n"))
	// The payload attempts to replace native path metadata. The guard must run
	// first and use the resolved filesystem path of the symlink, not that value.
	reserved := d.file("reserved.log", []byte("{\"message\":\"reserved-file-marker\",\"log.file.path_resolved\":\"/input-safe/safe.log\"}\n"))
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := &pathGuardSink{token: uuid.NewString()}
	server := grpc.NewServer()
	logsrpc.RegisterLogsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	id, host := uuid.NewString(), uuid.NewString()
	raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: id, ResourceID: host, ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: []configbundle.Collection{{ID: uuid.NewString(), Integration: "filelog", TargetType: "host", TargetID: host, Identity: json.RawMessage(`{}`), Config: json.RawMessage(`{"scope":"system","include":["/logs/*.log"],"start_at":"beginning","format":"json"}`), Enabled: true, Revision: 1, AssignmentEpoch: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	public, err := configbundle.Extract(raw, "host")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if json.Unmarshal(public, &config) != nil {
		t.Fatal("configuration")
	}
	config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/tmp/checkpoint", "create_directory": true}}
	config["service"].(map[string]any)["extensions"] = []string{"file_storage"}
	config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]any{"insecure": true}, "headers": map[string]string{"x-plan8-path-guard": sink.token}}}
	rendered, _ := json.Marshal(config)
	path := d.file("collector.json", rendered)
	container := d.start("collector", "--memory=384m", "--cpus=1", "--cap-drop=ALL", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--mount", "type=bind,source="+safe+",target=/input-safe/safe.log,readonly", "--mount", "type=bind,source="+reserved+",target=/var/lib/argus-otelcol/identity/reserved.log,readonly", "--entrypoint", "/bin/sh", "alpine:3.21", "-ec", `mkdir -p /logs; ln -s /input-safe/safe.log /logs/safe.log; ln -s /var/lib/argus-otelcol/identity/reserved.log /logs/reserved.log; exec /argus --config=/config.json`)
	deadline := time.Now().Add(20 * time.Second)
	for len(sink.snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	bodies := sink.snapshot()
	if len(bodies) != 1 || !strings.Contains(bodies[0], "safe-file-marker") {
		logs, _ := d.command("logs", "--tail", "15", container)
		t.Fatalf("resolved path guard: records=%d logs=%s", len(bodies), logs)
	}
	for _, body := range bodies {
		if strings.Contains(body, "reserved-file-marker") {
			t.Fatal("reserved runtime file reached the exporter")
		}
	}
	sink.mu.Lock()
	paths := append([]string{}, sink.paths...)
	sink.mu.Unlock()
	if len(paths) != 1 || paths[0] != "/input-safe/safe.log" {
		t.Fatalf("JSON payload changed native file lineage: %v", paths)
	}
	t.Log("native symlink paths verified; only the ordinary file reached the authenticated component sink")
}
