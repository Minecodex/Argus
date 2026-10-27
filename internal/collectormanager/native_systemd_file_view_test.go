package collectormanager

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

// Exercise the generated unit's mount policy with real systemd. The following
// formal file-log stage separately verifies actual native Collector samples.
func TestNativeSystemdFileLogViewPreservesHostPathsAndPrivateRuntime(t *testing.T) {
	image := os.Getenv("ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE")
	if image == "" {
		t.Skip("owned systemd image required")
	}
	f := newNativeResourceFixture(t)
	container := f.start("systemd-view", "--privileged", "--cgroupns", "private", "--memory=256m", "--cpus=1", "--tmpfs", "/run", "--tmpfs", "/run/lock", image, "/sbin/init")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := f.command("", "exec", container, "systemctl", "show-environment"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned systemd not ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.must("", "exec", container, "mkdir", "-p", "/var/lib/argus-otelcol", "/etc/argus-otelcol", "/tmp/argus-plan8-fs-probe")
	f.must("actual host log\n", "exec", "-i", container, "sh", "-c", "cat > /tmp/argus-plan8-fs-probe/input.log")
	script := `set -eu
if test "$1" = hidden; then
  test ! -r /tmp/argus-plan8-fs-probe/input.log
else
  test -r /tmp/argus-plan8-fs-probe/input.log
  if (printf unexpected > /tmp/argus-plan8-fs-probe/forbidden.log) 2>/dev/null; then exit 7; fi
fi
test -d "$TMPDIR"
test "$(stat -c %a "$TMPDIR")" = 700
touch "$TMPDIR/owned-internal-temp"
`
	f.must(script, "exec", "-i", container, "sh", "-c", "cat > /opt/argus-plan8-fs-probe.sh")
	results := []map[string]any{}
	for _, fileEnabled := range []bool{false, true} {
		bundle, err := configbundle.Render(configbundle.RenderInput{CollectorID: f.collector, ResourceID: f.host, ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestHTTPEndpoint: "https://ingest.test", IngestGRPCEndpoint: "grpcs://ingest.test:4317", ProfileKeys: []string{"host-basic"}, Collections: []configbundle.Collection{{ID: uuid.NewString(), Integration: "filelog", TargetType: "host", TargetID: f.host, Revision: 1, AssignmentEpoch: 1, Enabled: fileEnabled, Config: json.RawMessage(`{"scope":"system","include":["/tmp/argus-plan8-fs-probe/*.log"],"start_at":"beginning"}`)}}})
		if err != nil {
			t.Fatal(err)
		}
		command := &connectorv1.CollectorManagementCommand{RenderedConfig: bundle}
		unit := localCollectorSystemdUnit(command, "/var/lib/argus-otelcol")
		mode := "hidden"
		if fileEnabled {
			mode = "visible"
		}
		unit = strings.Replace(unit, "Type=simple", "Type=oneshot", 1)
		unit = strings.Replace(unit, "Restart=always", "Restart=no", 1)
		start := "ExecStart=/usr/local/bin/argus-otelcol --config=/etc/argus-otelcol/config.yaml --config=/etc/argus-otelcol/monitoring-private.json"
		if !strings.Contains(unit, start) {
			t.Fatal("generated unit start contract changed")
		}
		unit = strings.Replace(unit, start, "ExecStart=/bin/sh /opt/argus-plan8-fs-probe.sh "+mode, 1)
		f.must(unit, "exec", "-i", container, "sh", "-c", "cat > /etc/systemd/system/argus-plan8-fs-probe.service")
		f.must("", "exec", container, "systemctl", "daemon-reload")
		if _, err = f.command("", "exec", container, "systemctl", "start", "argus-plan8-fs-probe.service"); err != nil {
			journal, _ := f.command("", "exec", container, "journalctl", "--unit", "argus-plan8-fs-probe.service", "--no-pager", "--lines", "15")
			t.Fatalf("generated file-enabled=%t unit policy failed: %s", fileEnabled, journal)
		}
		f.must("", "exec", container, "test", "!", "-e", "/tmp/argus-plan8-fs-probe/forbidden.log")
		results = append(results, map[string]any{"filelog_enabled": fileEnabled, "host_tmp_visible": fileEnabled, "host_tmp_not_written": true, "internal_tmp_writable": true, "internal_tmp_mode": "0700"})
	}
	if path := os.Getenv("ARGUS_SYSTEMD_FILELOG_VIEW_EVIDENCE"); path != "" {
		body, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual_systemd_generated_mount_policy_not_collector_data_pipeline", "results": results}, "", "  ")
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
