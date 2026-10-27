package monitoringcatalog

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestCatalogEligibilityAndStrictConfiguration(t *testing.T) {
	entries := All()
	if len(entries) != 39 {
		t.Fatalf("catalog count %d", len(entries))
	}
	deferredCount := 0
	keys := map[string]bool{}
	for _, entry := range entries {
		if keys[entry.Key] {
			t.Fatal("duplicate integration")
		}
		keys[entry.Key] = true
		if entry.Selection == "deferred_no_eligible_embedded_component" {
			deferredCount++
			if entry.AdapterReady || entry.Component != "" || entry.Reason == "" {
				t.Fatal("deferred entry can compile")
			}
		}
		if entry.Validation == "passed" {
			t.Fatal("unverified catalog claims support")
		}
	}
	if deferredCount != 11 {
		t.Fatal("deferred list drift")
	}
	protocol, _ := Lookup("skywalking")
	if protocol.Selection != "deferred_upstream_defect" || protocol.AdapterReady || protocol.Validation != "blocked" {
		t.Fatal("user-deferred protocol became available or its failed admission was hidden")
	}
	for _, tc := range []struct{ key, identity, config string }{
		{"mysql", `{"endpoint":"db:3306","password":"no"}`, `{}`},
		{"mysql", `{"endpoint":"${env:SECRET}:3306"}`, `{}`},
		{"mysql", `{"endpoint":"db:3306"}`, `{"sql":"SELECT 1"}`},
		{"mysql", `{"endpoint":"db:3306"}`, `{"interval":"100ms"}`},
		{"nginx", `{"endpoint":"https://user:pass@nginx/status"}`, `{}`},
		{"nginx", `{"endpoint":"https://nginx/status?token=secret"}`, `{}`},
		{"redis", `{"endpoint":"redis:70000"}`, `{}`},
	} {
		if _, _, err := Parse(tc.key, []byte(tc.identity), []byte(tc.config)); err == nil {
			t.Fatalf("unsafe config accepted: %s", tc.key)
		}
	}
	entry, _ := Lookup("mysql")
	entry.Signals[0] = "changed"
	again, _ := Lookup("mysql")
	if !slices.Equal(again.Signals, []string{"metrics"}) {
		t.Fatal("mutable shared catalog")
	}
}

func TestPostgresFailedDiscoveryCannotEmitSyntheticInventoryCounts(t *testing.T) {
	config, err := ReceiverConfig("postgresql", uuid.New(), []byte(`{"endpoint":"db.example.test:5432","database":"postgres"}`), []byte(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	metrics := config["metrics"].(map[string]any)
	for _, name := range []string{"postgresql.database.count", "postgresql.table.count"} {
		if metrics[name].(map[string]any)["enabled"] != false {
			t.Fatal("failed-query zero count can falsely refresh target")
		}
	}
}

func TestNativeCollectorConfiguration(t *testing.T) {
	binary := os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY")
	if binary == "" {
		t.Skip("requires the built Linux Collector and Docker")
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	receivers := map[string]any{}
	names := []string{}
	privateReceivers := map[string]any{}
	for _, tc := range []struct{ key, endpoint string }{
		{"mysql", "mysql-a:3306"}, {"mysql", "mysql-b:3306"}, {"redis", "redis:6379"},
		{"postgresql", "postgres:5432"}, {"sqlserver", "sqlserver:1433"}, {"oracle", "oracle:1521"},
		{"mongodb", "mongodb:27017"}, {"memcached", "memcached:11211"}, {"kafka", "kafka:9092"},
		{"rabbitmq", "http://rabbitmq:15672"}, {"zookeeper", "zookeeper:2181"},
		{"nginx", "http://nginx:80/status"}, {"apache", "http://apache/server-status?auto"},
		{"haproxy", "http://haproxy:8404/stats"}, {"elasticsearch", "http://elasticsearch:9200"},
		{"http", "https://example.org/health"}, {"tcp", "example.org:443"}, {"tls", "example.org:443"},
		{"dns", "dns:53"}, {"icmp", "example.org"},
	} {
		id := uuid.New()
		target := TargetIdentity{Endpoint: tc.endpoint}
		if tc.key == "oracle" {
			target.Database = "FREEPDB1"
		}
		if tc.key == "dns" {
			target.Hostnames = []string{"example.org"}
		}
		identity, _ := json.Marshal(target)
		config, err := ReceiverConfig(tc.key, id, identity, []byte(`{}`), credentialsSupported(tc.key))
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := Lookup(tc.key)
		name := entry.Component + "/" + id.String()
		if credentialsSupported(tc.key) {
			overlay, err := CredentialOverlay(tc.key, identity, []byte(`{}`), []byte(`{"username":"monitor","password":"test-${env:HOME}-$literal"}`))
			if err != nil {
				t.Fatal(err)
			}
			privateReceivers[name] = overlay
		}
		receivers[name] = config
		names = append(names, name)
	}
	config := map[string]any{"receivers": receivers, "exporters": map[string]any{"otlp": map[string]any{"endpoint": "127.0.0.1:4317", "tls": map[string]bool{"insecure": true}}}, "service": map[string]any{"pipelines": map[string]any{"metrics": map[string]any{"receivers": names, "exporters": []string{"otlp"}}}}}
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(directory, "config.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	privateRaw, _ := json.Marshal(map[string]any{"receivers": privateReceivers})
	if err = os.WriteFile(filepath.Join(directory, "private.json"), privateRaw, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--read-only", "--network", "none", "--cap-drop", "ALL", "--label", "argus.owner=plan8-20261018", "--mount", "type=bind,source="+abs+",target=/argus,readonly", "--mount", "type=bind,source="+directory+",target=/test,readonly", "--entrypoint", "/argus", "alpine:3.21", "validate", "--config=/test/config.json", "--config=/test/private.json")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native configuration validation: %v\n%s", err, output)
	}
}
