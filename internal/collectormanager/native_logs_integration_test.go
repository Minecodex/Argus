package collectormanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type nativeLogRecord struct {
	body                 string
	resource, attributes map[string]string
}
type nativeLogCapture struct {
	logsrpc.UnimplementedLogsServiceServer
	mu      sync.Mutex
	token   string
	paused  bool
	records []nativeLogRecord
}

func stringAttributes(values []*commonpb.KeyValue) map[string]string {
	result := map[string]string{}
	for _, v := range values {
		result[v.Key] = v.Value.GetStringValue()
	}
	return result
}
func (c *nativeLogCapture) Export(ctx context.Context, r *logsrpc.ExportLogsServiceRequest) (*logsrpc.ExportLogsServiceResponse, error) {
	meta, _ := metadata.FromIncomingContext(ctx)
	if !slices.Equal(meta.Get("x-plan8-native-test"), []string{c.token}) {
		return nil, status.Error(codes.Unauthenticated, "test receiver authorization")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused {
		return nil, status.Error(codes.Unavailable, "test sink temporarily unavailable")
	}
	for _, resource := range r.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			for _, record := range scope.LogRecords {
				c.records = append(c.records, nativeLogRecord{record.Body.GetStringValue(), stringAttributes(resource.Resource.GetAttributes()), stringAttributes(record.Attributes)})
			}
		}
	}
	return &logsrpc.ExportLogsServiceResponse{}, nil
}
func (c *nativeLogCapture) snapshot() []nativeLogRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.records)
}

func (c *nativeLogCapture) setPaused(paused bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = paused
}

// Actual native receiver/checkpoint gate. This sink is deliberately separate
// from the required deployed platform ingestion, authorization and query gates.
func TestNativeFileLogsRotationCheckpointAndIsolation(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_LOGS") != "1" {
		t.Skip("requires Docker and the built Linux Collector")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if info, e := os.Stat(binary); e != nil || info.IsDir() {
		t.Fatal("Collector binary required")
	}
	dir := t.TempDir()
	owner := "plan8-logs-" + uuid.NewString()[:8]
	for _, sub := range []string{"files", "checkpoint"} {
		if err = os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	docker := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(file, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "files", file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	appendLine := func(file, content string) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(dir, "files", file), os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(content)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write: %v %v", err, closeErr)
		}
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	capture := &nativeLogCapture{token: uuid.NewString()}
	server := grpc.NewServer()
	logsrpc.RegisterLogsServiceServer(server, capture)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	collector, host, app, service := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	collections := []configbundle.Collection{
		{ID: uuid.NewString(), Integration: "filelog", TargetType: "application", TargetID: app, Revision: 1, AssignmentEpoch: 1, Enabled: true, Service: &configbundle.CollectionService{ID: service, Name: "orders", Namespace: "business", Environment: "production"}, Config: json.RawMessage(`{"scope":"business","include":["/logs/orders*.log","/logs/literal$path.log"],"exclude":["/logs/orders-excluded.log"],"start_at":"beginning","format":"json"}`)},
		{ID: uuid.NewString(), Integration: "filelog", TargetType: "host", TargetID: host, Revision: 1, AssignmentEpoch: 1, Enabled: true, Config: json.RawMessage(`{"scope":"system","include":["/logs/system.log"],"start_at":"beginning"}`)},
		{ID: uuid.NewString(), Integration: "filelog", TargetType: "host", TargetID: host, Revision: 1, AssignmentEpoch: 1, Enabled: true, Config: json.RawMessage(`{"scope":"component","include":["/logs/multiline.log"],"start_at":"beginning","multiline_start":"^START"}`)},
	}
	line := func(event string) string {
		b, _ := json.Marshal(map[string]string{"event": event, "message": strings.Repeat(event+" ", 160)})
		return string(b) + "\n"
	}
	write("orders.log", line("first"))
	write("orders-excluded.log", line("excluded"))
	write("literal$path.log", line("literal"))
	write("system.log", "system-first "+strings.Repeat("system ", 160)+"\n")
	write("multiline.log", "START stack\n  at orders\n  caused by test\n")
	run := func(revision int64) string {
		t.Helper()
		raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: host, ResourceType: "host", Platform: "linux_amd64", Role: "leaf", SourceGeneration: collector, ConfigRevision: revision, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if err != nil {
			t.Fatal(err)
		}
		hostConfig, err := configbundle.Extract(raw, "host")
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err = json.Unmarshal(hostConfig, &config); err != nil {
			t.Fatal(err)
		}
		config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/checkpoint", "create_directory": true}}
		config["service"].(map[string]any)["extensions"] = []string{"file_storage"}
		config["processors"].(map[string]any)["batch"] = map[string]any{"timeout": "200ms"}
		config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-native-test": capture.token}}}
		public, _ := json.Marshal(config)
		configPath := filepath.Join(dir, fmt.Sprintf("config-%d.json", revision))
		if err = os.WriteFile(configPath, public, 0600); err != nil {
			t.Fatal(err)
		}
		id := docker("run", "-d", "--name", fmt.Sprintf("%s-%d", owner, revision), "--label", "argus.owner="+owner, "--memory=384m", "--cpus=1", "--read-only", "--cap-drop=ALL", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+configPath+",target=/config.json,readonly", "--mount", "type=bind,source="+filepath.Join(dir, "files")+",target=/logs,readonly", "--mount", "type=bind,source="+filepath.Join(dir, "checkpoint")+",target=/checkpoint", "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json")
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			label, e := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, id).Output()
			if e == nil && strings.TrimSpace(string(label)) == owner {
				_ = exec.CommandContext(ctx, "docker", "rm", "-f", "-v", id).Run()
			}
		})
		return id
	}
	wait := func(collectorID string, count int) []nativeLogRecord {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if records := capture.snapshot(); len(records) >= count {
				return records
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("expected %d records, received %d: %s", count, len(capture.snapshot()), docker("logs", "--tail", "25", collectorID))
		return nil
	}
	first := run(1)
	records := wait(first, 4)
	for _, record := range records {
		if record.attributes["event"] == "excluded" {
			t.Fatal("excluded file exported")
		}
		if record.resource["argus.collection.id"] == collections[0].ID {
			for k, v := range map[string]string{"argus.resource.type": "application", "argus.resource.id": app, "argus.application.id": app, "argus.service.id": service, "service.name": "orders", "deployment.environment.name": "production"} {
				if record.resource[k] != v {
					t.Fatalf("business identity %s: %q", k, record.resource[k])
				}
			}
		} else if record.resource["argus.resource.id"] != host || record.resource["argus.application.id"] != "" {
			t.Fatal("system and business identities merged")
		}
		if record.resource["argus.source.id"] == "" || record.attributes["log.file.path"] == "" {
			t.Fatal("missing provenance")
		}
	}
	if !slices.ContainsFunc(records, func(r nativeLogRecord) bool { return r.body == "START stack\n  at orders\n  caused by test" }) {
		t.Fatal("multiline record split")
	}
	canary := "plan8-invalid-json-private-canary"
	appendLine("orders.log", "invalid-json "+canary+"\n")
	wait(first, 5)
	if strings.Contains(docker("logs", first), canary) {
		t.Fatal("parse error exposed the raw log record")
	}
	if err = os.Rename(filepath.Join(dir, "files", "orders.log"), filepath.Join(dir, "files", "orders.log.1")); err != nil {
		t.Fatal(err)
	}
	write("orders.log", line("rotated"))
	wait(first, 6)
	docker("stop", "--time", "10", first)
	appendLine("orders.log", line("after-restart"))
	second := run(2)
	wait(second, 7)
	// Allow several poll/flush intervals to detect checkpoint replay, not just arrival.
	time.Sleep(2 * time.Second)
	if got := len(capture.snapshot()); got != 7 {
		t.Fatalf("checkpoint replay: got %d records", got)
	}
	docker("stop", "--time", "10", second)
	collections[0].Enabled = false
	collections[0].Revision = 2
	appendLine("orders.log", line("disabled"))
	appendLine("system.log", "system-survives\n")
	third := run(3)
	wait(third, 8)
	time.Sleep(2 * time.Second)
	records = capture.snapshot()
	if len(records) != 8 || !slices.ContainsFunc(records, func(r nativeLogRecord) bool { return r.body == "system-survives" }) {
		t.Fatalf("independent disable: %d records", len(records))
	}
	for _, r := range records {
		if r.attributes["event"] == "disabled" || r.attributes["event"] == "excluded" {
			t.Fatal("disabled or excluded data collected")
		}
	}
	t.Logf("native filelog: %d records; business/system isolation, JSON, multiline, literal path, exclusion, rotation, persisted checkpoint and independent disable verified", len(records))
}
