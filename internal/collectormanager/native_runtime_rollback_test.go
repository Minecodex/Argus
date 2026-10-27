package collectormanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Additional orchestration of the frozen production SSH activation script.
// Docker transports the exact script/stdin; this does not claim SSH enrollment.
func TestOwnedActualCollectorActivationRollbackAndRevisionFence(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_ACTUAL_ROLLBACK") != "1" {
		t.Skip("explicit owned native rollback execution required")
	}
	output := os.Getenv("ARGUS_PLAN8_ACTUAL_ROLLBACK_EVIDENCE")
	if output == "" {
		t.Fatal("specific evidence path required")
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatal("refuse to replace an existing rollback observation")
	}
	complete := false
	proof := map[string]any{"passed": false, "scope": "actual_native_collector_systemd_and_production_activation_script_with_controlled_docker_transport_and_ca_not_formal_enrollment", "started_at": time.Now().UTC()}
	t.Cleanup(func() {
		proof["passed"] = complete && !t.Failed() && !t.Skipped()
		proof["finished_at"] = time.Now().UTC()
		body, _ := json.MarshalIndent(proof, "", "  ")
		if err := os.WriteFile(output, body, 0600); err != nil {
			t.Error(err)
		}
	})
	f := newNativeResourceFixture(t)
	image := os.Getenv("ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE")
	if image == "" {
		t.Fatal("owned systemd image required")
	}
	container := f.start("rollback", "--privileged", "--cgroupns", "private", "--memory=512m", "--cpus=1", "--tmpfs", "/run", "--tmpfs", "/run/lock", image, "/sbin/init")
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	run := func(input []byte, args ...string) (string, error) {
		c := exec.CommandContext(ctx, "docker", args...)
		c.Stdin = bytes.NewReader(input)
		b, e := c.CombinedOutput()
		return strings.TrimSpace(string(b)), e
	}
	must := func(input []byte, args ...string) string {
		t.Helper()
		b, e := run(input, args...)
		if e != nil {
			t.Fatalf("owned rollback operation failed: %v: %s", e, b)
		}
		return b
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, e := run(nil, "exec", container, "systemctl", "show-environment"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("systemd not ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
	must(nil, "exec", container, "mkdir", "-p", "/var/lib/argus-otelcol/identity", "/etc/argus-otelcol", "/opt/plan8-release")
	identity := t.TempDir()
	writeTestLocalCollectorIdentity(t, identity, f.collector)
	must(nil, "cp", filepath.Join(identity, "identity")+string(os.PathSeparator)+".", container+":/var/lib/argus-otelcol/identity/")
	must(nil, "cp", f.binary, container+":/opt/plan8-release/argus-otelcol")
	must(nil, "exec", container, "chmod", "755", "/opt/plan8-release/argus-otelcol")
	must(nil, "exec", container, "tar", "-czf", "/opt/plan8-release/artifact.tar.gz", "-C", "/opt/plan8-release", "argus-otelcol")
	binary, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	proof["collector_sha256"] = hex.EncodeToString(sum[:])
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, &ownedCoexistMetricCapture{nativeMetricCapture: f.capture, allowed: map[string]string{f.host: "host"}})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	source := uuid.NewString()
	config := func(mode string, revision int) []byte {
		receivers := map[string]any{"hostmetrics": map[string]any{"collection_interval": "2s", "scrapers": map[string]any{"cpu": map[string]any{}, "memory": map[string]any{}}}}
		used := []string{"hostmetrics"}
		if mode == "startup_failure" {
			receivers["otlp"] = map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:18444"}}}
			used = append(used, "otlp")
		}
		if mode == "validation_failure" {
			receivers["not_a_registered_receiver"] = map[string]any{}
			used = append(used, "not_a_registered_receiver")
		}
		attributes := []map[string]any{}
		for key, value := range map[string]string{"argus.resource.id": f.host, "argus.resource.type": "host", "argus.source.id": source, "plan8.test.config_revision": fmt.Sprint(revision)} {
			attributes = append(attributes, map[string]any{"key": key, "value": value, "action": "upsert"})
		}
		body, _ := json.Marshal(map[string]any{"extensions": map[string]any{"health_check": map[string]any{"endpoint": "127.0.0.1:13133"}}, "receivers": receivers, "processors": map[string]any{"resource/owned": map[string]any{"attributes": attributes}}, "exporters": map[string]any{"otlp/owned": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-native-test": f.capture.token}}}, "service": map[string]any{"extensions": []string{"health_check"}, "pipelines": map[string]any{"metrics": map[string]any{"receivers": used, "processors": []string{"resource/owned"}, "exporters": []string{"otlp/owned"}}}}})
		return body
	}
	good := config("good", 1)
	newer := config("good", 3)
	prepare := func(revision int, body []byte) string {
		stage := fmt.Sprintf("/var/lib/argus-otelcol/%s/candidate-%d-owned-rollback", f.collector, revision)
		must(nil, "exec", container, "mkdir", "-p", stage+"/release")
		must(nil, "exec", container, "cp", "/opt/plan8-release/artifact.tar.gz", stage+"/artifact.tar.gz")
		hash := sha256.Sum256(body)
		unit := localCollectorSystemdUnit(&connectorv1.CollectorManagementCommand{}, "/var/lib/argus-otelcol")
		ca, e := os.ReadFile(filepath.Join(identity, "identity", "ca.pem"))
		if e != nil {
			t.Fatal(e)
		}
		files := map[string][]byte{"config.json": body, "service.unit": []byte(unit), "server-ca.pem": ca, "revision": []byte(fmt.Sprintf("%d\n", revision)), "config-hash": []byte(hex.EncodeToString(hash[:]) + "\n"), "collector-id": []byte(f.collector + "\n")}
		var archive bytes.Buffer
		tw := tar.NewWriter(&archive)
		for name, value := range files {
			if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(value))}); e != nil {
				t.Fatal(e)
			}
			if _, e := tw.Write(value); e != nil {
				t.Fatal(e)
			}
		}
		if e := tw.Close(); e != nil {
			t.Fatal(e)
		}
		must(archive.Bytes(), "exec", "-i", container, "tar", "-xf", "-", "-C", stage)
		return stage
	}
	activate := func(stage string) (string, error) {
		var p bytes.Buffer
		tw := tar.NewWriter(&p)
		for _, name := range []string{"private.json", "enrollment-token"} {
			value := []byte("{}")
			if name == "enrollment-token" {
				value = nil
			}
			_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(value))})
			_, _ = tw.Write(value)
		}
		_ = tw.Close()
		return run(p.Bytes(), "exec", "-i", container, "sh", "-c", sshActivateCommand(stage))
	}
	samples := func(revision string) string {
		t.Helper()
		invocation := must(nil, "exec", container, "systemctl", "show", "argus-otelcol.service", "-p", "InvocationID", "--value")
		if len(invocation) != 32 {
			t.Fatal("running invocation identity missing")
		}
		deadline := time.Now().Add(15 * time.Second)
		since := uint64(time.Now().UnixNano())
		for {
			fresh := false
			f.capture.mu.Lock()
			for _, request := range f.capture.requests {
				for _, resource := range request.ResourceMetrics {
					matched := false
					for _, a := range resource.Resource.GetAttributes() {
						matched = matched || (a.Key == "plan8.test.config_revision" && a.Value.GetStringValue() == revision)
					}
					if !matched {
						continue
					}
					for _, scope := range resource.ScopeMetrics {
						for _, metric := range scope.Metrics {
							if metric.Name != "system.cpu.time" {
								continue
							}
							for _, point := range metric.GetSum().GetDataPoints() {
								fresh = fresh || point.TimeUnixNano > since
							}
						}
					}
				}
			}
			f.capture.mu.Unlock()
			if fresh {
				return invocation
			}
			if time.Now().After(deadline) {
				t.Fatal("current process did not produce an actual host metric")
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
	before := prepare(1, good)
	if b, e := activate(before); e != nil {
		must([]byte("{}"), "exec", "-i", container, "sh", "-c", "cat > "+before+"/private.json")
		diagnostic, _ := run(nil, "exec", container, before+"/release/argus-otelcol", "validate", "--config="+before+"/config.json", "--config="+before+"/private.json")
		t.Log("actual baseline validation diagnostic:", diagnostic)
		t.Fatalf("baseline activation: %v %s", e, b)
	}
	proof["baseline_sample_invocation"] = samples("1")
	baselinePID := must(nil, "exec", container, "systemctl", "show", "argus-otelcol.service", "-p", "MainPID", "--value")
	invalid := prepare(2, config("validation_failure", 2))
	if _, e := activate(invalid); e == nil {
		t.Fatal("actual binary accepted an unknown receiver")
	}
	if now := must(nil, "exec", container, "systemctl", "show", "argus-otelcol.service", "-p", "MainPID", "--value"); now != baselinePID {
		t.Fatal("validation failure replaced running baseline")
	}
	proof["actual_binary_validation_failure_preserved_process"] = true
	must(nil, "exec", "-d", container, "systemd-socket-activate", "-l", "127.0.0.1:18444", "/bin/cat")
	bad := prepare(2, config("startup_failure", 2))
	third := prepare(3, newer)
	type result struct {
		body string
		err  error
	}
	failed := make(chan result, 1)
	go func() { b, e := activate(bad); failed <- result{b, e} }()
	faultDeadline := time.Now().Add(20 * time.Second)
	for {
		b, _ := run(nil, "exec", container, "journalctl", "-u", "argus-otelcol.service", "--no-pager", "--lines", "80")
		if strings.Contains(b, "address already in use") {
			break
		}
		if time.Now().After(faultDeadline) {
			t.Fatal("actual listener startup failure was not observed")
		}
		time.Sleep(200 * time.Millisecond)
	}
	// The activation lock must reject a conflicting attempt; it must not modify
	// another in-progress activation or allow its eventual rollback to erase it.
	if _, e := activate(third); e == nil {
		t.Fatal("concurrent revision bypassed an active rollback lock")
	}
	r := <-failed
	if r.err == nil {
		t.Fatal("startup failure reported success")
	}
	if strings.Contains(r.body, "ERROR") {
		proof["startup_failure_diagnostic_present"] = true
	}
	must(nil, "exec", container, "systemctl", "is-active", "--quiet", "argus-otelcol.service")
	if rev := must(nil, "exec", container, "cat", "/var/lib/argus-otelcol/.effective-revision"); rev != "1" {
		t.Fatalf("bad revision published: %s", rev)
	}
	actual := must(nil, "exec", container, "sha256sum", "/etc/argus-otelcol/config.yaml")
	expected := sha256.Sum256(good)
	if !strings.HasPrefix(actual, hex.EncodeToString(expected[:])) {
		t.Fatal("rollback did not restore exact prior configuration")
	}
	proof["rollback_sample_invocation"] = samples("1")
	proof["actual_startup_failure_rolled_back_configuration_and_running_samples"] = true
	proof["concurrent_attempt_rejected_without_write"] = true
	if b, e := activate(third); e != nil {
		t.Fatalf("newer revision retry did not converge: %v %s", e, b)
	}
	stale := prepare(1, good)
	if _, e := activate(stale); e == nil {
		t.Fatal("stale revision replaced newer revision")
	}
	if rev := must(nil, "exec", container, "cat", "/var/lib/argus-otelcol/.effective-revision"); rev != "3" {
		t.Fatal("stale rollback overwrote newer effective revision")
	}
	actual = must(nil, "exec", container, "sha256sum", "/etc/argus-otelcol/config.yaml")
	expected = sha256.Sum256(newer)
	if !strings.HasPrefix(actual, hex.EncodeToString(expected[:])) {
		t.Fatal("newer config hash changed after stale attempt")
	}
	proof["newer_sample_invocation"] = samples("3")
	proof["newer_revision_preserved_after_stale_attempt"] = true
	script := sha256.Sum256([]byte(sshActivateCommand("fixed-owned-stage")))
	proof["activation_script_sha256"] = hex.EncodeToString(script[:])
	proof["validation_failure_did_not_restart_old_pid"] = true
	proof["final_effective_revision"] = 3
	proof["actual_native_metrics_payloads"] = f.capture.payloads()
	complete = true
}
