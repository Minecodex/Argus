package collectormanager

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNativeMemcachedApacheAndHAProxyTargets(t *testing.T) {
	f := newNativeResourceFixture(t)
	f.start("memcached", "--memory=64m", "--cpus=.5", "memcached:1.6-alpine", "memcached", "-m", "16", "-c", "32", "-t", "1")
	base := f.must("", "run", "--rm", "--label", "argus.owner="+f.owner, "--entrypoint", "cat", "httpd:2.4-alpine", "/usr/local/apache2/conf/httpd.conf")
	if !strings.Contains(base, "LoadModule status_module modules/mod_status.so") {
		t.Fatal("owned Apache image lacks loaded mod_status prerequisite")
	}
	base = strings.Replace(base, "Listen 80", "Listen 8080", 1) + "\nServerName localhost\nExtendedStatus On\n<Location /server-status>\nSetHandler server-status\nRequire all granted\n</Location>\n"
	path := f.write("httpd.conf", []byte(base))
	apache := f.start("apache", "--memory=96m", "--cpus=.5", "--mount", "type=bind,source="+path+",target=/usr/local/apache2/conf/httpd.conf,readonly", "httpd:2.4-alpine")
	password := "plan8-statistics-password"
	config := "global\n  maxconn 64\ndefaults\n  mode http\n  timeout connect 2s\n  timeout client 5s\n  timeout server 5s\nfrontend public\n  bind *:8081\n  default_backend applications\nbackend applications\n  server apache " + f.owner + "-apache:8080 check\nlisten stats\n  bind *:8080\n  mode http\n  stats enable\n  stats uri /stats\n  stats auth monitor:" + password + "\n"
	haproxyPath := f.write("haproxy.cfg", []byte(config))
	haproxy := f.start("haproxy", "--memory=96m", "--cpus=.5", "--mount", "type=bind,source="+haproxyPath+",target=/usr/local/etc/haproxy/haproxy.cfg,readonly", "haproxy:3.2-alpine")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := f.command("", "exec", apache, "wget", "-q", "-O", "/dev/null", "http://"+f.owner+"-haproxy:8081/server-status?auto"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned Apache/HAProxy endpoints not ready")
		}
		time.Sleep(300 * time.Millisecond)
	}
	f.add("memcached", f.owner+"-memcached:11211", "", nil)
	f.add("apache", "http://"+f.owner+"-apache:8080/server-status?auto", "", nil)
	f.add("haproxy", "http://"+f.owner+"-haproxy:8080/stats", "", map[string]string{"username": "monitor", "password": password})
	first := f.run(1)
	samples := f.wait(first, 3)
	for i := 0; i < 4; i++ {
		f.must("", "exec", apache, "wget", "-q", "-O", "/dev/null", "http://"+f.owner+"-haproxy:8081/server-status?auto")
		time.Sleep(time.Second)
	}
	wanted := [][]string{{"memcached.bytes", "memcached.connections.current", "memcached.current_items", "memcached.commands", "memcached.evictions", "memcached.network"}, {"apache.requests", "apache.uptime", "apache.workers", "apache.traffic", "apache.scoreboard", "apache.current_connections"}, {"haproxy.bytes.input", "haproxy.bytes.output", "haproxy.sessions.count", "haproxy.requests.total"}}
	for i, collection := range f.collections {
		for _, name := range wanted[i] {
			if !slices.Contains(samples[collection.TargetID], name) {
				t.Fatalf("native %s omitted %s: %v", collection.Integration, name, samples[collection.TargetID])
			}
		}
	}
	f.save(os.Getenv("ARGUS_CACHE_WEB_METRIC_EVIDENCE"), 1)
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	f.credentials[f.collections[2].ID] = map[string]string{"username": "monitor", "password": "wrong-statistics-password"}
	second := f.run(2)
	f.wait(second, 2)
	time.Sleep(3 * time.Second)
	if f.capture.snapshot()[f.collections[2].TargetID] != nil {
		t.Fatal("wrong HAProxy auth produced target data")
	}
	f.must("", "stop", "--time", "1", second)
	f.capture.reset()
	f.credentials[f.collections[2].ID] = map[string]string{"username": "monitor", "password": password}
	f.collections[0].Enabled = false
	f.collections[0].Revision = 2
	third := f.run(3)
	f.wait(third, 2)
	if f.capture.snapshot()[f.collections[0].TargetID] != nil || f.capture.snapshot()[f.collections[2].TargetID] == nil {
		t.Fatal("cache disable or HAProxy credential recovery failed")
	}
	f.must("", "stop", "--time", "1", third)
	f.capture.reset()
	for i := range f.collections {
		f.collections[i].Enabled = false
		f.collections[i].Revision++
	}
	fourth := f.run(4)
	time.Sleep(2 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", fourth) != "true" {
		t.Fatal("all cache/web targets disabled but control runtime incorrect")
	}
	if path := os.Getenv("ARGUS_CACHE_WEB_TARGET_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(map[string]any{"scope": "native_dedicated_components_not_platform_enrollment", "metrics": samples, "haproxy_container": haproxy, "auth_recovery": "passed", "independent_disable": "passed"}, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("actual Memcached/Apache mod_status/HAProxy CSV: native metrics, HTTP traffic, read-only statistics authentication failure/recovery, independent disable and all-off runtime")
}
