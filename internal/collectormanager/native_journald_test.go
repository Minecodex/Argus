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
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

func TestNativeJournaldReadsActualJournalctlAndCheckpoints(t *testing.T) {
	image := os.Getenv("ARGUS_JOURNALD_TEST_IMAGE")
	if image == "" {
		t.Skip("requires an isolated Linux image with systemd-journald and journalctl")
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
	sink := &pathGuardSink{token: uuid.NewString()}
	server := grpc.NewServer()
	logsrpc.RegisterLogsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	collector, host := uuid.NewString(), uuid.NewString()
	raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: []configbundle.Collection{{ID: uuid.NewString(), Integration: "journald", TargetType: "host", TargetID: host, Config: json.RawMessage(`{"scope":"system","start_at":"beginning","format":"text"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true}}})
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
	config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/checkpoint", "create_directory": true}}
	config["service"].(map[string]any)["extensions"] = []string{"file_storage"}
	config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]any{"insecure": true}, "headers": map[string]string{"x-plan8-path-guard": sink.token}, "sending_queue": map[string]any{"enabled": true, "storage": "file_storage", "queue_size": 1024}}}
	value, _ := json.Marshal(config)
	configPath := d.file("journal-collector.json", value)
	container := d.start("journal", "--memory=512m", "--cpus=1", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+configPath+",target=/config.json,readonly", "--entrypoint", "/bin/sh", image, "-ec", `mkdir -p /run/systemd/journal /var/log/journal /checkpoint; systemd-machine-id-setup; /usr/lib/systemd/systemd-journald >/tmp/journal-daemon.log 2>&1 & exec sleep infinity`)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = d.command("exec", container, "test", "-S", "/run/systemd/journal/socket"); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	marker := "plan8-native-journal-" + uuid.NewString()
	d.must("exec", container, "journalctl", "--flush")
	d.must("exec", container, "systemd-cat", "--identifier=plan8-native", "/bin/echo", marker+"-first")
	d.must("exec", container, "journalctl", "--sync")
	d.must("exec", "-d", container, "/bin/sh", "-c", "exec /argus --config=/config.json >/tmp/collector.log 2>&1")
	wait := func(wanted string) {
		t.Helper()
		end := time.Now().Add(20 * time.Second)
		for time.Now().Before(end) {
			for _, body := range sink.snapshot() {
				if strings.Contains(body, wanted) {
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		logs, _ := d.command("logs", "--tail", "15", container)
		collectorLog, _ := d.command("exec", container, "cat", "/tmp/collector.log")
		diagnostic, _ := d.command("exec", container, "journalctl", "--no-pager", "-n", "4")
		t.Fatalf("native journal sample missing; container=%s collector=%s journal=%s", logs, collectorLog, diagnostic)
	}
	wait(marker + "-first")
	sink.pause(true)
	d.must("exec", container, "systemd-cat", "--priority=debug", "--identifier=plan8-native", "/bin/echo", marker+"-debug")
	time.Sleep(2 * time.Second)
	d.must("exec", container, "pkill", "-KILL", "-x", "argus")
	time.Sleep(time.Second)
	d.must("exec", container, "systemd-cat", "--identifier=plan8-native", "/bin/echo", marker+"-second")
	d.must("exec", "-d", container, "/bin/sh", "-c", "exec /argus --config=/config.json >/tmp/collector.log 2>&1")
	sink.pause(false)
	wait(marker + "-second")
	wait(marker + "-debug")
	counts := map[string]int{}
	for _, body := range sink.snapshot() {
		for _, suffix := range []string{"first", "debug", "second"} {
			if strings.Contains(body, marker+"-"+suffix) {
				counts[suffix]++
			}
		}
	}
	if counts["first"] != 1 || counts["debug"] != 1 || counts["second"] != 1 {
		for _, body := range sink.snapshot() {
			if strings.Contains(body, marker) {
				t.Logf("matching journal record: %s", body)
			}
		}
		collectorLog, _ := d.command("exec", container, "cat", "/tmp/collector.log")
		storage, _ := d.command("exec", container, "ls", "-la", "/checkpoint")
		t.Fatalf("journal checkpoint replay: %+v; storage=%s collector=%s", counts, storage, collectorLog)
	}
	t.Log("actual journalctl/native receiver: info and debug preserved; sink outage plus KILL resumed from persistent queue and cursor")
}
