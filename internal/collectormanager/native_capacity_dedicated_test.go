package collectormanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Dedicated receivers and HTTP endpoints are real. This uses the existing
// controlled OTLP capture and does not claim enrollment, queries or full A62.
func TestNativeDedicatedHTTP32TargetFrozenCapacity(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_CAPACITY_PROFILE") != "local-linux-amd64-v1" {
		t.Skip("explicit frozen profile required")
	}
	f := newNativeResourceFixture(t)
	var failures [32]atomic.Bool
	var requests [32]atomic.Uint64
	for i := 0; i < 32; i++ {
		listener, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		index := i
		server := &http.Server{ReadHeaderTimeout: 2 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests[index].Add(1)
			if failures[index].Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		})}
		go server.Serve(listener)
		t.Cleanup(func() { _ = server.Close() })
		f.add("http", fmt.Sprintf("http://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "", nil)
	}
	collector := f.run(1)
	f.wait(collector, 32)
	rss := func() uint64 {
		t.Helper()
		raw := f.must("", "exec", collector, "cat", "/proc/1/status")
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				return value * 1024
			}
		}
		t.Fatal("Collector RSS unavailable")
		return 0
	}
	initialRSS, peakRSS := rss(), uint64(0)
	observations := []uint64{}
	started := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for i := 0; i < 60; i++ {
		select {
		case <-ticker.C:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
		current := rss()
		observations = append(observations, current)
		if current > peakRSS {
			peakRSS = current
		}
		if current > 384<<20 {
			t.Fatal("Collector exceeded frozen 384 MiB threshold", current)
		}
	}
	steadyMillis := time.Since(started).Milliseconds()
	counts := []uint64{}
	for i := range requests {
		if requests[i].Load() == 0 {
			t.Fatal("target was never actually requested", i)
		}
		counts = append(counts, requests[i].Load())
		f.waitValue(i, "httpcheck.status", map[string]string{"http.status_class": "2xx"}, 1)
	}
	failures[7].Store(true)
	f.capture.reset()
	faultAt := time.Now()
	nativeCapacityHTTPStatusAfter(f, 7, "5xx", faultAt)
	nativeCapacityHTTPStatusAfter(f, 8, "2xx", faultAt)
	failures[7].Store(false)
	f.capture.reset()
	nativeCapacityHTTPStatusAfter(f, 7, "2xx", time.Now())
	f.must("", "stop", "--time", "1", collector)
	f.capture.reset()
	f.collections[0].Enabled, f.collections[0].Revision = false, 2
	second := f.run(2)
	f.wait(second, 31)
	if f.capture.snapshot()[f.collections[0].TargetID] != nil {
		t.Fatal("disabled capacity target still produced samples")
	}
	f.must("", "stop", "--time", "1", second)
	f.capture.reset()
	for i := range f.collections {
		f.collections[i].Enabled, f.collections[i].Revision = false, 3
	}
	third := f.run(3)
	time.Sleep(4 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", third) != "true" {
		t.Fatal("all-off capacity runtime was not idle and running")
	}
	profile, err := os.ReadFile("../../artifacts/plan8-capacity-profile-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	profileHash, binaryHash := sha256.Sum256(profile), sha256.Sum256(binary)
	receipt := map[string]any{"passed": true, "verification_level": "actual_native_http_receivers_controlled_otlp_capture_not_formal_enrollment_or_full_A62", "profile_id": "plan8-local-linux-amd64-v1", "profile_sha256": hex.EncodeToString(profileHash[:]), "collector_sha256": hex.EncodeToString(binaryHash[:]), "target_count": 32, "distinct_http_listeners": 32, "steady_duration_ms": steadyMillis, "actual_request_counts": counts, "initial_rss_bytes": initialRSS, "peak_rss_bytes": peakRSS, "rss_observations_bytes": observations, "one_target_fault_and_recovery": true, "independent_disable": true, "all_off_runtime_running": true, "remaining": []string{"dedicated_target_formal_enrollment_and_queries", "persistent_queue_disk_capacity", "network_tls_outage", "quota_pressure", "full_formal_load"}}
	if path := os.Getenv("ARGUS_PLAN8_DEDICATED_CAPACITY_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual 32 dedicated HTTP receivers: steady=%dms peakRSS=%d; formal and queue fault gates remain", steadyMillis, peakRSS)
}

func nativeCapacityHTTPStatusAfter(f *nativeResourceFixture, index int, class string, after time.Time) {
	f.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		found := func() bool {
			f.capture.mu.Lock()
			defer f.capture.mu.Unlock()
			for _, request := range f.capture.requests {
				for _, r := range request.ResourceMetrics {
					target := ""
					for _, a := range r.Resource.GetAttributes() {
						if a.Key == "argus.resource.id" {
							target = a.Value.GetStringValue()
						}
					}
					if target != f.collections[index].TargetID {
						continue
					}
					for _, scope := range r.ScopeMetrics {
						for _, metric := range scope.Metrics {
							if metric.Name != "httpcheck.status" {
								continue
							}
							for _, point := range append(metric.GetGauge().GetDataPoints(), metric.GetSum().GetDataPoints()...) {
								if point.TimeUnixNano <= uint64(after.UnixNano()) || point.GetAsInt() != 1 && point.GetAsDouble() != 1 {
									continue
								}
								for _, a := range point.Attributes {
									if a.Key == "http.status_class" && a.Value.GetStringValue() == class {
										return true
									}
								}
							}
						}
					}
				}
			}
			return false
		}()
		if found {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.t.Fatalf("target %d has no actual new %s sample after transition", index, class)
}
