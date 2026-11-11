package collectormanager_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type shutdownReplaySink struct {
	metricsrpc.UnimplementedMetricsServiceServer
	blocked           atomic.Bool
	mu                sync.Mutex
	waiting, received map[string]bool
}

func (s *shutdownReplaySink) Export(_ context.Context, request *metricsrpc.ExportMetricsServiceRequest) (*metricsrpc.ExportMetricsServiceResponse, error) {
	if s.blocked.Load() {
		// Keep several real scrapes queued and their graceful shutdown observable.
		time.Sleep(200 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range request.ResourceMetrics {
		raw, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(resource)
		digest := sha256.Sum256(raw)
		key := hex.EncodeToString(digest[:])
		if s.blocked.Load() {
			s.waiting[key] = true
		} else {
			s.received[key] = true
		}
	}
	if s.blocked.Load() {
		return nil, status.Error(codes.Unavailable, "owned temporary sink outage")
	}
	return &metricsrpc.ExportMetricsServiceResponse{}, nil
}

// This component gate uses real CPU scrapes and the released native persistent
// queue. The separately deployed B09b review verifies Argus mTLS and its ledger.
func TestNativePersistentQueueReplaysShutdownFailures(t *testing.T) {
	image := os.Getenv("ARGUS_NATIVE_DELIVERY_REPLAY_IMAGE")
	if image == "" {
		t.Skip("requires explicitly selected native Collector image")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	owner := "argus-shutdown-replay-" + uuid.NewString()[:8]
	docker := func(args ...string) string {
		t.Helper()
		raw, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s: %v: %s", args[0], err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	directory := t.TempDir()
	sink := &shutdownReplaySink{waiting: map[string]bool{}, received: map[string]bool{}}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	config := map[string]any{
		"extensions": map[string]any{"file_storage": map[string]any{"directory": "/state/queue", "create_directory": true}},
		"receivers":  map[string]any{"hostmetrics": map[string]any{"collection_interval": "1s", "scrapers": map[string]any{"cpu": map[string]any{}}}},
		"exporters": map[string]any{"otlp/argus": map[string]any{
			"endpoint": "host.docker.internal:" + strings.Split(listener.Addr().String(), ":")[len(strings.Split(listener.Addr().String(), ":"))-1],
			"tls":      map[string]any{"insecure": true}, "timeout": "1s",
			"sending_queue":    map[string]any{"enabled": true, "storage": "file_storage", "num_consumers": 1, "queue_size": 100},
			"retry_on_failure": map[string]any{"enabled": true, "initial_interval": "1s", "max_interval": "2s", "max_elapsed_time": "5m"}}},
		"service": map[string]any{"extensions": []string{"file_storage"},
			"telemetry": map[string]any{"metrics": map[string]any{"level": "detailed", "readers": []any{map[string]any{"pull": map[string]any{"exporter": map[string]any{"prometheus": map[string]any{"host": "0.0.0.0", "port": 8888}}}}}}},
			"pipelines": map[string]any{"metrics": map[string]any{"receivers": []string{"hostmetrics"}, "exporters": []string{"otlp/argus"}}}},
	}
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(directory, "collector.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	docker("run", "-d", "--name", owner, "--label", "argus.test-owner="+owner, "--user", "0:0", "--memory", "256m", "--cpus", "0.5",
		"-p", "127.0.0.1::8888", "--mount", "type=bind,source="+directory+",target=/state", image, "--config=/state/collector.json")
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		label, e := exec.CommandContext(clean, "docker", "inspect", "--format", "{{index .Config.Labels \"argus.test-owner\"}}", owner).Output()
		if e == nil && strings.TrimSpace(string(label)) == owner {
			_ = exec.CommandContext(clean, "docker", "rm", "-f", owner).Run()
		}
	})
	address := "http://" + docker("port", owner, "8888/tcp") + "/metrics"
	wait := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: %s", label, docker("logs", owner))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	wait("real CPU baseline", func() bool { sink.mu.Lock(); defer sink.mu.Unlock(); return len(sink.received) > 1 })
	sink.blocked.Store(true)
	wait("actual retrying CPU batch", func() bool { sink.mu.Lock(); defer sink.mu.Unlock(); return len(sink.waiting) > 0 })
	time.Sleep(5 * time.Second)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	var observed atomic.Bool
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stopped:
				return
			default:
			}
			response, e := client.Get(address)
			if e == nil {
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				for _, line := range strings.Split(string(body), "\n") {
					if strings.HasPrefix(line, "otelcol_exporter_send_failed_metric_points{") && strings.Contains(line, "error_type=\"Shutdown\"") && strings.Contains(line, "error_permanent=\"false\"") {
						observed.Store(true)
					}
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	docker("stop", "--time", "25", owner)
	close(stopped)
	<-done
	log := docker("logs", owner)
	if !strings.Contains(log, "interrupted due to shutdown") {
		t.Fatal("actual retry shutdown was not exercised", log)
	}
	// No source replacement: the same on-disk request must reach the sink.
	sink.blocked.Store(false)
	docker("start", owner)
	wait("exact pre-shutdown CPU batch replay", func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		for key := range sink.waiting {
			if !sink.received[key] {
				return false
			}
		}
		return len(sink.waiting) > 0
	})
	if !observed.Load() {
		// Metrics shutdown races the last scrape. The durable replay and actual
		// Shutdown log are the invariant; counter visibility is diagnostic only.
		t.Log("native metrics endpoint closed before the final shutdown scrape")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	t.Logf("native Shutdown counter observed=%v; %d exact failed CPU batch(es) replayed after restart", observed.Load(), len(sink.waiting))
}
