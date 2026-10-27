package collectormanager

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"
)

func TestNativeDNSAndICMPActualTargets(t *testing.T) {
	f := newNativeResourceFixture(t)
	corefile := f.write("Corefile", []byte(".:1053 {\n hosts {\n  192.0.2.42 reachable.plan8.test\n  ttl 1\n }\n}\n"))
	dns := f.start("dns", "--memory=64m", "--cpus=.5", "--read-only", "--cap-drop=ALL", "--cap-add=NET_BIND_SERVICE", "--mount", "type=bind,source="+corefile+",target=/Corefile,readonly", "coredns/coredns:1.12.4", "-conf", "/Corefile")
	peer := f.start("ping-peer", "--memory=16m", "--cpus=.25", "--cap-drop=ALL", "alpine:3.21", "sleep", "300")
	ip := f.must("", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", peer)
	for _, protocol := range []string{"udp", "tcp"} {
		f.add("dns", f.owner+"-dns:1053", "", nil)
		index := len(f.collections) - 1
		identity, _ := json.Marshal(map[string]any{"endpoint": f.owner + "-dns:1053", "hostnames": []string{"reachable.plan8.test", "missing.plan8.test"}})
		f.collections[index].Identity = identity
		cfg, _ := json.Marshal(map[string]any{"interval": "3s", "timeout": "2s", "dns_network": protocol, "record_type": "A"})
		f.collections[index].Config = cfg
	}
	f.add("icmp", ip, "", nil)
	f.collections[2].Config = json.RawMessage(`{"interval":"3s","timeout":"2s","ping_count":2}`)
	options := []string{"--user=65532:65532", "--sysctl", "net.ipv4.ping_group_range=0 2147483647"}
	first := f.runOptions(1, options...)
	f.wait(first, 3)
	for i := range 2 {
		f.waitValue(i, "dnscheck.status", map[string]string{"dns.domain": "reachable.plan8.test", "dns.resolved.ip": "192.0.2.42"}, 1)
		f.waitValue(i, "dnscheck.status", map[string]string{"dns.domain": "missing.plan8.test"}, 0)
	}
	samples := f.capture.snapshot()
	for i, names := range [][]string{{"dnscheck.duration", "dnscheck.status"}, {"dnscheck.duration", "dnscheck.status"}, {"ping.loss.ratio", "ping.rtt.min", "ping.rtt.max", "ping.rtt.avg", "ping.rtt.stddev"}} {
		for _, name := range names {
			if !slices.Contains(samples[f.collections[i].TargetID], name) {
				t.Fatalf("native %s missing metric %s", f.collections[i].Integration, name)
			}
		}
	}
	f.waitValue(2, "ping.loss.ratio", nil, 0)
	time.Sleep(4 * time.Second)
	f.save(os.Getenv("ARGUS_NETWORK_PROBE_METRIC_EVIDENCE"), 1)
	// Removing the actual peer produces packet loss, not a fabricated metric.
	f.must("", "stop", "--time", "1", peer)
	f.waitValue(2, "ping.loss.ratio", nil, 100)
	f.save(os.Getenv("ARGUS_NETWORK_PROBE_FAILURE_EVIDENCE"), 1)
	f.must("", "start", peer)
	if restored := f.must("", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", peer); restored != ip {
		t.Fatal("ping peer identity changed on restart")
	}
	f.waitValue(2, "ping.loss.ratio", nil, 0)
	f.must("", "stop", "--time", "1", dns)
	for i := range 2 {
		// Filter by domain only: failed responses must not retain a resolved IP.
		f.waitValue(i, "dnscheck.status", map[string]string{"dns.domain": "reachable.plan8.test"}, 0)
	}
	f.must("", "start", dns)
	for i := range 2 {
		f.waitValue(i, "dnscheck.status", map[string]string{"dns.domain": "reachable.plan8.test", "dns.resolved.ip": "192.0.2.42"}, 1)
	}
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	// A restricted ping socket range removes ICMP capability. DNS still works;
	// the upstream ICMP receiver must not manufacture healthy zero-loss data.
	denied := f.runOptions(2, "--user=65532:65532", "--sysctl", "net.ipv4.ping_group_range=1 0")
	f.wait(denied, 2)
	time.Sleep(4 * time.Second)
	if f.capture.snapshot()[f.collections[2].TargetID] != nil {
		t.Fatal("ICMP permission denial fabricated target data")
	}
	f.must("", "stop", "--time", "1", denied)
	f.capture.reset()
	f.collections[0].Enabled, f.collections[0].Revision = false, 2
	recovered := f.runOptions(3, options...)
	f.wait(recovered, 2)
	f.waitValue(2, "ping.loss.ratio", nil, 0)
	if f.capture.snapshot()[f.collections[0].TargetID] != nil {
		t.Fatal("disabled UDP DNS still emitted target data")
	}
	f.must("", "stop", "--time", "1", recovered)
	f.capture.reset()
	for i := range f.collections {
		f.collections[i].Enabled = false
		f.collections[i].Revision++
	}
	allOff := f.runOptions(4, options...)
	time.Sleep(3 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", allOff) != "true" {
		t.Fatal("all-off network probe runtime incorrect")
	}
	t.Log("actual CoreDNS UDP/TCP A and missing name, DNS failure/recovery, ICMP packet loss/recovery, non-root cap-drop permission refusal/recovery, independent disable and all-off runtime")
}
