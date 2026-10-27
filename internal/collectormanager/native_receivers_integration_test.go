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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type nativeMetricCapture struct {
	metricsrpc.UnimplementedMetricsServiceServer
	mu       sync.Mutex
	token    string
	targets  map[string]map[string]bool
	requests []*metricsrpc.ExportMetricsServiceRequest
}

func (c *nativeMetricCapture) Export(ctx context.Context, r *metricsrpc.ExportMetricsServiceRequest) (*metricsrpc.ExportMetricsServiceResponse, error) {
	meta, _ := metadata.FromIncomingContext(ctx)
	if !slices.Equal(meta.Get("x-plan8-native-test"), []string{c.token}) {
		return nil, status.Error(codes.Unauthenticated, "test receiver authorization")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range r.ResourceMetrics {
		target := ""
		kind := ""
		source := ""
		for _, a := range resource.Resource.GetAttributes() {
			switch a.Key {
			case "argus.resource.id":
				target = a.Value.GetStringValue()
			case "argus.resource.type":
				kind = a.Value.GetStringValue()
			case "argus.source.id":
				source = a.Value.GetStringValue()
			}
		}
		if target == "" || kind != "monitoring_target" || source == "" {
			return nil, status.Error(codes.InvalidArgument, "missing native target identity")
		}
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if len(metric.GetGauge().GetDataPoints())+len(metric.GetSum().GetDataPoints())+len(metric.GetHistogram().GetDataPoints())+len(metric.GetExponentialHistogram().GetDataPoints())+len(metric.GetSummary().GetDataPoints()) == 0 {
					continue
				}
				if c.targets[target] == nil {
					c.targets[target] = map[string]bool{}
				}
				c.targets[target][metric.Name] = true
			}
		}
	}
	c.requests = append(c.requests, proto.Clone(r).(*metricsrpc.ExportMetricsServiceRequest))
	return &metricsrpc.ExportMetricsServiceResponse{}, nil
}
func (c *nativeMetricCapture) snapshot() map[string][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := map[string][]string{}
	for target, names := range c.targets {
		for name := range names {
			result[target] = append(result[target], name)
		}
		sort.Strings(result[target])
	}
	return result
}
func (c *nativeMetricCapture) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targets = map[string]map[string]bool{}
	c.requests = nil
}

func (c *nativeMetricCapture) payloads() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([][]byte, len(c.requests))
	for i, request := range c.requests {
		result[i], _ = proto.Marshal(request)
	}
	return result
}

// This is the native component DATA gate. It intentionally uses an authenticated
// test OTLP sink; enterprise ingestion, storage and dashboard gates remain separate.
func TestNativeReceiversRealTargets(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_TARGETS") != "1" {
		t.Skip("requires disposable Docker targets and a built Linux Collector")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if info, e := os.Stat(binary); e != nil || info.IsDir() {
		t.Fatal("built Collector binary required")
	}
	owner := "plan8-native-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	directory := t.TempDir()
	docker := func(input string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "docker", args...)
		if input != "" {
			command.Stdin = strings.NewReader(input)
		}
		out, err := command.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	must := func(args ...string) string {
		t.Helper()
		out, err := docker("", args...)
		if err != nil {
			t.Fatalf("Docker test operation failed: %v: %s", err, out)
		}
		return out
	}
	network := must("network", "create", "--label", "argus.owner="+owner, owner)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "docker", "network", "rm", network).Run()
	})
	start := func(name string, args ...string) string {
		t.Helper()
		base := []string{"run", "-d", "--name", owner + "-" + name, "--label", "argus.owner=" + owner, "--network", network}
		id := must(append(base, args...)...)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			label, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, id).Output()
			if err == nil && strings.TrimSpace(string(label)) == owner {
				_ = exec.CommandContext(ctx, "docker", "rm", "-f", "-v", id).Run()
			}
		})
		return id
	}
	write := func(name string, value []byte) string {
		t.Helper()
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	password := "plan8-test-${env:HOME}-$literal"
	for _, name := range []string{"mysql-a", "mysql-b"} {
		start(name, "--memory=768m", "--cpus=1", "-e", "MYSQL_ROOT_PASSWORD="+password, "mysql:8.4", "--innodb-buffer-pool-size=64M", "--max-connections=30")
	}
	redisConfig := write("redis.conf", []byte("bind 0.0.0.0\nprotected-mode no\nappendonly no\nsave \"\"\nrequirepass \""+password+"\"\n"))
	start("redis", "--memory=64m", "--cpus=.5", "--mount", "type=bind,source="+redisConfig+",target=/redis.conf,readonly", "redis:7.4-alpine", "redis-server", "/redis.conf")
	nginxConfig := write("nginx.conf", []byte("worker_processes 1; events {worker_connections 128;} http {server {listen 8080; location /status {stub_status;}}}"))
	start("nginx", "--memory=64m", "--cpus=.5", "--mount", "type=bind,source="+nginxConfig+",target=/etc/nginx/nginx.conf,readonly", "nginx:1.28-alpine")
	for _, name := range []string{"mysql-a", "mysql-b"} {
		deadline := time.Now().Add(90 * time.Second)
		for {
			_, err := docker("SELECT 1;", "exec", "-i", "-e", "MYSQL_PWD="+password, owner+"-"+name, "mysql", "-uroot")
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not become ready", name)
			}
			time.Sleep(time.Second)
		}
		sql := "CREATE USER 'monitor'@'%' IDENTIFIED BY '" + password + "'; GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'monitor'@'%'; GRANT SELECT ON performance_schema.* TO 'monitor'@'%';"
		if out, err := docker(sql, "exec", "-i", "-e", "MYSQL_PWD="+password, owner+"-"+name, "mysql", "-uroot"); err != nil {
			t.Fatalf("monitor account setup: %v %s", err, out)
		}
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	capture := &nativeMetricCapture{token: uuid.NewString(), targets: map[string]map[string]bool{}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, capture)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	collectorID, hostID := uuid.NewString(), uuid.NewString()
	collections := []configbundle.Collection{}
	for _, item := range []struct{ key, endpoint string }{{"mysql", owner + "-mysql-a:3306"}, {"mysql", owner + "-mysql-b:3306"}, {"redis", owner + "-redis:6379"}, {"nginx", "http://" + owner + "-nginx:8080/status"}} {
		identity, _ := json.Marshal(map[string]string{"endpoint": item.endpoint})
		collections = append(collections, configbundle.Collection{ID: uuid.NewString(), Integration: item.key, TargetID: uuid.NewString(), TargetType: "monitoring_target", Identity: identity, Config: json.RawMessage(`{"interval":"2s","timeout":"2s"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true, HasCredential: item.key != "nginx"})
	}
	runCollector := func(revision int64) string {
		t.Helper()
		raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collectorID, ResourceID: hostID, ResourceType: "host", Role: "leaf", SourceGeneration: uuid.NewSHA1(uuid.NameSpaceOID, []byte(owner)).String(), ConfigRevision: revision, Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if err != nil {
			t.Fatal(err)
		}
		host, err := configbundle.Extract(raw, "host")
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		_ = json.Unmarshal(host, &config)
		delete(config, "extensions")
		service := config["service"].(map[string]any)
		delete(service, "extensions")
		config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-native-test": capture.token}}}
		public, _ := json.Marshal(config)
		publicPath := write(fmt.Sprintf("public-%d.json", revision), public)
		privateReceivers := map[string]map[string]string{}
		for _, item := range collections {
			if item.Enabled && item.HasCredential {
				username := "monitor"
				if item.Integration == "redis" {
					username = "default"
				}
				privateReceivers[item.Integration+"/collection_"+strings.ReplaceAll(item.ID, "-", "")] = map[string]string{"username": username, "password": password}
			}
		}
		privateInput, _ := json.Marshal(map[string]any{"receivers": privateReceivers})
		private, err := monitoringPrivateConfig(&connectorv1.CollectorManagementCommand{RenderedConfig: raw, PrivateMonitoringConfig: privateInput})
		if err != nil {
			t.Fatal(err)
		}
		privatePath := write(fmt.Sprintf("private-%d.json", revision), private)
		clear(private)
		return start(fmt.Sprintf("collector-%d", revision), "--memory=512m", "--cpus=1", "--read-only", "--cap-drop=ALL", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+publicPath+",target=/config.json,readonly", "--mount", "type=bind,source="+privatePath+",target=/private.json,readonly", "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json", "--config=/private.json")
	}
	waitTargets := func(expected int, collector string) map[string][]string {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			samples := capture.snapshot()
			if len(samples) == expected {
				return samples
			}
			time.Sleep(time.Second)
		}
		logs, _ := docker("", "logs", "--tail", "30", collector)
		t.Fatalf("native metric targets: expected %d got %v\n%s", expected, capture.snapshot(), logs)
		return nil
	}
	first := runCollector(1)
	firstSamples := waitTargets(4, first)
	// Rates require multiple real observations, never synthesized timestamps.
	time.Sleep(8 * time.Second)
	firstPayloads := capture.payloads()
	if path := os.Getenv("ARGUS_MONITORING_NATIVE_PAYLOADS"); path != "" {
		raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collectorID, ResourceID: hostID, ResourceType: "host", Role: "leaf", SourceGeneration: uuid.NewSHA1(uuid.NameSpaceOID, []byte(owner)).String(), ConfigRevision: 1, Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if err != nil {
			t.Fatal(err)
		}
		sources, err := configbundle.Sources(raw)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.MarshalIndent(map[string]any{"scope": "authenticated_native_sink_not_platform_e2e", "captured_at": time.Now().UTC(), "collector_id": collectorID, "host_id": hostID, "sources": sources, "requests": firstPayloads}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range collections {
		names := firstSamples[item.TargetID]
		if len(names) == 0 {
			t.Fatalf("no metrics for %s", item.Integration)
		}
		t.Logf("%s target=%s metric_names=%d first=%s", item.Integration, item.TargetID, len(names), names[0])
	}
	must("stop", "--time", "3", first)
	capture.reset()
	collections[0].Enabled = false
	collections[0].Revision = 2
	second := runCollector(2)
	secondSamples := waitTargets(3, second)
	if _, exists := secondSamples[collections[0].TargetID]; exists {
		t.Fatal("disabled MySQL still exported")
	}
	if _, exists := secondSamples[collections[1].TargetID]; !exists {
		t.Fatal("second MySQL stopped with the first")
	}
	if destination := os.Getenv("ARGUS_MONITORING_NATIVE_EVIDENCE"); destination != "" {
		images := map[string]string{}
		for _, name := range []string{"mysql:8.4", "redis:7.4-alpine", "nginx:1.28-alpine", "alpine:3.21"} {
			images[name] = must("image", "inspect", "--format", "{{json .RepoDigests}}", name)
		}
		result, _ := json.MarshalIndent(map[string]any{"scope": "native_components_not_platform_e2e", "collector_version": configbundle.CollectorVersion, "images": images, "first_revision": firstSamples, "second_revision": secondSamples, "disabled_target": collections[0].TargetID, "literal_password_verified": true}, "", "  ")
		if err = os.WriteFile(destination, result, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
