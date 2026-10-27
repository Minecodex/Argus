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

func TestNativeJournaldFiltersRealSystemdUnits(t *testing.T) {
	image := os.Getenv("ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE")
	if image == "" {
		t.Skip("requires an explicitly selected disposable full-systemd container")
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
	app, host := uuid.NewString(), uuid.NewString()
	collection := configbundle.Collection{ID: uuid.NewString(), Integration: "journald", TargetType: "application", TargetID: app, Config: json.RawMessage(`{"scope":"business","units":["plan8-primary"],"start_at":"beginning"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true, Service: &configbundle.CollectionService{ID: uuid.NewString(), Name: "primary", Environment: "testing"}}
	raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: uuid.NewString(), ResourceID: host, ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: []configbundle.Collection{collection}})
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
	config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-path-guard": sink.token}}}
	value, _ := json.Marshal(config)
	path := d.file("unit-collector.json", value)
	marker := "plan8-real-unit-" + uuid.NewString()
	unit := func(suffix string) string {
		return d.file(suffix+".service", []byte("[Unit]\nDescription=Plan8 isolated journal unit\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/bin/echo "+marker+"-"+suffix+"\nStandardOutput=journal\n"))
	}
	primary, other := unit("primary"), unit("other")
	// A private cgroup namespace and owned container provide actual systemd unit
	// attribution. No host directory, service or cgroup tree is mounted writable.
	container := d.start("systemd", "--memory=768m", "--cpus=1", "--privileged", "--cgroupns=private", "--tmpfs", "/run", "--tmpfs", "/run/lock", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--mount", "type=bind,source="+primary+",target=/etc/systemd/system/plan8-primary.service,readonly", "--mount", "type=bind,source="+other+",target=/etc/systemd/system/plan8-other.service,readonly", "--entrypoint", "/sbin/init", image)
	deadline := time.Now().Add(20 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if output, e := d.command("exec", container, "systemctl", "is-active", "systemd-journald"); e == nil && output == "active" {
			ready = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		logs, _ := d.command("logs", "--tail", "12", container)
		t.Fatalf("isolated systemd did not start: %s", logs)
	}
	d.must("exec", container, "journalctl", "--flush")
	d.must("exec", container, "systemctl", "start", "plan8-primary.service", "plan8-other.service")
	d.must("exec", "-d", container, "/bin/sh", "-c", "exec /argus --config=/config.json >/tmp/collector.log 2>&1")
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		for _, body := range sink.snapshot() {
			if strings.Contains(body, marker+"-other") {
				t.Fatal("foreign systemd unit crossed the collection rule")
			}
			if strings.Contains(body, marker+"-primary") {
				if !strings.Contains(body, "plan8-primary.service") {
					t.Fatal("journal record lost actual systemd attribution")
				}
				found = true
			}
		}
		if found {
			time.Sleep(2 * time.Second)
			for _, body := range sink.snapshot() {
				if strings.Contains(body, marker+"-other") {
					t.Fatal("foreign systemd unit arrived after selected unit")
				}
			}
			t.Log("actual systemd units: selected business service collected, second unit excluded by native journalctl filter")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	logs, _ := d.command("exec", container, "cat", "/tmp/collector.log")
	t.Fatalf("selected systemd journal missing: %s", logs)
}
