package collectormanager_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (s *builtinMetricSink) latestSystemdState(source, unit, state string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value int64
	var latest uint64
	for _, request := range s.requests {
		for _, resource := range request.ResourceMetrics {
			matchedSource, matchedUnit := false, false
			for _, a := range resource.Resource.GetAttributes() {
				if a.Key == "argus.source.id" && a.Value.GetStringValue() == source {
					matchedSource = true
				}
				if a.Key == "systemd.unit.name" && a.Value.GetStringValue() == unit {
					matchedUnit = true
				}
			}
			if !matchedSource || !matchedUnit {
				continue
			}
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name != "systemd.unit.state" {
						continue
					}
					for _, p := range metric.GetSum().GetDataPoints() {
						for _, a := range p.Attributes {
							if a.Key == "systemd.unit.active_state" && a.Value.GetStringValue() == state && uint64(p.TimeUnixNano) > latest {
								latest = uint64(p.TimeUnixNano)
								value = p.GetAsInt()
							}
						}
					}
				}
			}
		}
	}
	return value, latest > 0
}

func TestNativeHostProcessAndRealSystemdServiceMetrics(t *testing.T) {
	image := os.Getenv("ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE")
	if image == "" {
		t.Skip("requires explicitly selected disposable full-systemd container")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	d := newPipelineDocker(t)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := &builtinMetricSink{token: uuid.NewString(), samples: map[string]map[string]string{}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	host, collector, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
	collections := []configbundle.Collection{}
	for _, key := range []string{"host_process_metrics", "host_systemd_metrics"} {
		collections = append(collections, configbundle.Collection{ID: uuid.NewString(), Integration: key, TargetType: "host", TargetID: host, Config: json.RawMessage(`{"interval":"5s"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true})
	}
	config := func(revision int64) (string, []configbundle.Source) {
		t.Helper()
		raw, e := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "direct", Platform: "linux_amd64", SourceGeneration: generation, ConfigRevision: revision, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if e != nil {
			t.Fatal(e)
		}
		sources, e := configbundle.Sources(raw)
		if e != nil {
			t.Fatal(e)
		}
		public, e := configbundle.Extract(raw, "host")
		if e != nil {
			t.Fatal(e)
		}
		var conf map[string]any
		if e = json.Unmarshal(public, &conf); e != nil {
			t.Fatal(e)
		}
		conf["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/tmp/argus-storage", "create_directory": true}, "health_check": map[string]any{"endpoint": "127.0.0.1:13133"}}
		conf["service"].(map[string]any)["extensions"] = []string{"file_storage", "health_check"}
		conf["exporters"].(map[string]any)["otlp/argus"] = map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]any{"insecure": true}, "headers": map[string]string{"x-plan8-builtin": sink.token}}
		value, e := json.Marshal(conf)
		if e != nil {
			t.Fatal(e)
		}
		return d.file("process-services.json", value), sources
	}
	path, sources := config(1)
	canary := "plan8-private-command-" + uuid.NewString()
	activeUnit := d.file("active.service", []byte("[Unit]\nDescription=Plan8 disposable active service\n[Service]\nType=simple\nExecStart=/bin/sh -c 'sleep 120' "+canary+"\nCPUAccounting=yes\n"))
	failedUnit := d.file("failed.service", []byte("[Unit]\nDescription=Plan8 disposable failing service\n[Service]\nType=oneshot\nExecStart=/bin/false\n"))
	// Systemd may unload an unreferenced stopped unit. A real dependency keeps
	// this observed unit loaded so native inactive state can be asserted.
	keeperUnit := d.file("keeper.service", []byte("[Unit]\nDescription=Plan8 unit dependency\nWants=plan8-active.service\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/bin/true\n"))
	container := d.start("host-services", "--memory=768m", "--cpus=1", "--privileged", "--cgroupns=private", "--tmpfs", "/run", "--tmpfs", "/run/lock", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--mount", "type=bind,source="+activeUnit+",target=/etc/systemd/system/plan8-active.service,readonly", "--mount", "type=bind,source="+failedUnit+",target=/etc/systemd/system/plan8-failed.service,readonly", "--mount", "type=bind,source="+keeperUnit+",target=/etc/systemd/system/plan8-keeper.service,readonly", "--entrypoint", "/sbin/init", image)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if value, e := d.command("exec", container, "systemctl", "is-active", "systemd-journald"); e == nil && value == "active" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("isolated systemd did not start")
		}
		time.Sleep(200 * time.Millisecond)
	}
	d.must("exec", container, "systemctl", "start", "dbus.service", "plan8-keeper.service")
	if _, e := d.command("exec", container, "systemctl", "start", "plan8-failed.service"); e == nil {
		t.Fatal("controlled failed service unexpectedly succeeded")
	}
	start := func() {
		d.must("exec", "-d", container, "/bin/sh", "-c", "echo $$ >/tmp/argus.pid; exec /argus --config=/config.json >/tmp/collector.log 2>&1")
	}
	stop := func() {
		d.must("exec", container, "/bin/sh", "-c", "kill -TERM $(cat /tmp/argus.pid)")
		time.Sleep(2 * time.Second)
	}
	start()
	serviceSource, processSource := "", ""
	for _, source := range sources {
		if source.Type == "host_systemd" {
			serviceSource = source.ID
		}
		if source.Type == "host_processes" {
			processSource = source.ID
		}
	}
	waitState := func(state string) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			a, ok := sink.latestSystemdState(serviceSource, "plan8-active.service", state)
			b, failed := sink.latestSystemdState(serviceSource, "plan8-failed.service", "failed")
			if ok && a == 1 && failed && b == 1 && sink.snapshot()[processSource]["process.memory.usage"] != "" {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		logs, _ := d.command("exec", container, "tail", "-n", "15", "/tmp/collector.log")
		t.Fatalf("actual systemd/process state %s missing: %s", state, logs)
	}
	waitState("active")
	d.must("exec", container, "systemctl", "stop", "plan8-active.service")
	waitState("inactive")
	// Save actual, unmodified Resource blocks before later configuration epochs.
	sink.mu.Lock()
	requests := append([]*metricsrpc.ExportMetricsServiceRequest{}, sink.requests...)
	sink.mu.Unlock()
	payloads := make([][]byte, len(requests))
	for i, request := range requests {
		raw, e := proto.Marshal(request)
		if e != nil {
			t.Fatal(e)
		}
		payloads[i] = raw
		for _, resource := range request.ResourceMetrics {
			for _, a := range resource.Resource.GetAttributes() {
				if a.Key == "process.command_line" || a.Key == "process.command" || a.Key == "process.executable.path" || a.Key == "process.owner" {
					t.Fatalf("private process identity %s emitted", a.Key)
				}
			}
		}
		if strings.Contains(request.String(), canary) {
			t.Fatal("command-line canary crossed native capture")
		}
	}
	samples := sink.snapshot()
	for _, name := range []string{"process.cpu.time", "process.memory.usage", "process.memory.virtual", "process.disk.io"} {
		if samples[processSource][name] == "" {
			t.Fatalf("actual process metric missing: %s", name)
		}
	}
	for _, name := range []string{"systemd.unit.state", "systemd.service.cpu.time", "systemd.service.restarts"} {
		if samples[serviceSource][name] == "" {
			t.Fatalf("actual service metric missing: %s", name)
		}
	}
	if evidence := os.Getenv("ARGUS_PROCESS_SERVICE_METRIC_EVIDENCE"); evidence != "" {
		raw, _ := json.MarshalIndent(map[string]any{"scope": "actual_linux_container_process_and_systemd_not_formal_host", "collector_version": configbundle.CollectorVersion, "captured_at": time.Now().UTC(), "collector_id": collector, "host_id": host, "sources": sources, "metric_names_and_units": samples, "requests": payloads}, "", "  ")
		if err = os.WriteFile(evidence, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	sink.reset()
	collections[1].Enabled = false
	collections[1].Revision = 2
	_, remaining := config(2)
	start()
	deadline = time.Now().Add(15 * time.Second)
	for len(sink.snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if len(remaining) != 1 || len(sink.snapshot()) != 1 || sink.snapshot()[serviceSource] != nil {
		t.Fatal("service disable affected process independence")
	}
	stop()
	sink.reset()
	collections[0].Enabled = false
	collections[0].Revision = 2
	_, remaining = config(3)
	start()
	time.Sleep(6 * time.Second)
	if len(remaining) != 0 || len(sink.snapshot()) != 0 {
		t.Fatal("all-off produced implicit process/service samples")
	}
	d.must("exec", container, "/bin/sh", "-c", "kill -0 $(cat /tmp/argus.pid)")
	t.Log("actual processes and systemd active/failed/stopped states, no command arguments, independent disable and all-off runtime passed; container scope only")
}
