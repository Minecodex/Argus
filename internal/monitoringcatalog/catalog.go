// Package monitoringcatalog defines the shared, versioned collection contract.
// It contains no storage, authorization or execution policy. A catalog entry is
// not a support claim: production admission also requires a tested distribution.
package monitoringcatalog

import "slices"

const Version = "argus.monitoring_catalog/v1"
const CollectorVersion = "0.159.0"

type Entry struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Family        string   `json:"family"`
	Component     string   `json:"component,omitempty"`
	Module        string   `json:"module,omitempty"`
	Signals       []string `json:"signals"`
	Platforms     []string `json:"platforms"`
	Selection     string   `json:"selection"`
	Validation    string   `json:"validation"`
	Reason        string   `json:"reason,omitempty"`
	ConfigVersion string   `json:"config_version"`
	AdapterReady  bool     `json:"adapter_ready"`
	Form          *Form    `json:"form,omitempty"`
}

// All returns independent values so request-local filtering cannot mutate the
// catalog used by another tenant or by the configuration compiler.
func All() []Entry {
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		entry.Signals = slices.Clone(entry.Signals)
		entry.Platforms = slices.Clone(entry.Platforms)
		if entry.Form != nil {
			copy := *entry.Form
			entry.Form = &copy
		}
		result = append(result, entry)
	}
	return result
}

func Lookup(key string) (Entry, bool) {
	for _, entry := range All() {
		if entry.Key == key {
			return entry, true
		}
	}
	return Entry{}, false
}

func native(key, name, family, component, module string) Entry {
	form := nativeForm(key)
	return Entry{Key: key, Name: name, Family: family, Component: component,
		Module:  "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/" + module,
		Signals: []string{"metrics"}, Platforms: []string{"linux_amd64", "linux_arm64", "windows_amd64"},
		Selection: "candidate", Validation: "not_run", ConfigVersion: Version,
		AdapterReady: family != "ingestion", Form: &form}
}

func deferred(key, name, family, reason string) Entry {
	return Entry{Key: key, Name: name, Family: family, Selection: "deferred_no_eligible_embedded_component", Validation: "not_applicable", Reason: reason, ConfigVersion: Version, Signals: []string{"metrics"}, Platforms: []string{}}
}

func ingestion(key, name, component, module string, signals ...string) Entry {
	e := native(key, name, "ingestion", component, module)
	e.AdapterReady = false
	if key == "filelog" || key == "k8slog" || key == "journald" {
		e.AdapterReady = true
	}
	e.Form = nil
	if key == "skywalking" {
		e.Selection = "deferred_upstream_defect"
		e.Validation = "blocked"
		e.Reason = "upstream_acknowledgement_and_payload_logging"
	}
	e.Signals = signals
	if key == "otlp" || key == "sdklog" {
		e.Module = "go.opentelemetry.io/collector/receiver/otlpreceiver"
	}
	if key == "journald" || key == "k8slog" {
		e.Platforms = []string{"linux_amd64", "linux_arm64"}
	}
	if key == "eventlog" {
		e.Platforms = []string{"windows_amd64"}
	}
	return e
}

var entries = []Entry{
	native("mysql", "MySQL", "database", "mysql", "mysqlreceiver"),
	native("postgresql", "PostgreSQL", "database", "postgresql", "postgresqlreceiver"),
	native("sqlserver", "SQL Server", "database", "sqlserver", "sqlserverreceiver"),
	native("oracle", "Oracle", "database", "oracledb", "oracledbreceiver"),
	native("mongodb", "MongoDB", "database", "mongodb", "mongodbreceiver"),
	deferred("clickhouse", "ClickHouse", "database", "no_dedicated_receiver"),
	deferred("tidb", "TiDB", "database", "mysql_compatibility_is_not_tidb_collection"),
	deferred("oceanbase", "OceanBase", "database", "no_dedicated_receiver"),
	native("redis", "Redis", "cache", "redis", "redisreceiver"),
	native("memcached", "Memcached", "cache", "memcached", "memcachedreceiver"),
	native("kafka", "Kafka", "messaging", "kafka_metrics", "kafkametricsreceiver"),
	native("rabbitmq", "RabbitMQ", "messaging", "rabbitmq", "rabbitmqreceiver"),
	deferred("rocketmq", "RocketMQ", "messaging", "no_dedicated_receiver"),
	deferred("pulsar", "Pulsar", "messaging", "receiver_transports_telemetry_not_broker_metrics"),
	deferred("nats", "NATS", "messaging", "no_eligible_mature_embedded_receiver"),
	native("zookeeper", "ZooKeeper", "messaging", "zookeeper", "zookeeperreceiver"),
	native("nginx", "Nginx", "web", "nginx", "nginxreceiver"),
	native("apache", "Apache HTTP Server", "web", "apache", "apachereceiver"),
	func() Entry {
		e := native("iis", "IIS", "web", "iis", "iisreceiver")
		e.Platforms = []string{"windows_amd64"}
		return e
	}(),
	native("haproxy", "HAProxy", "web", "haproxy", "haproxyreceiver"),
	deferred("envoy", "Envoy", "web", "access_log_receiver_is_not_resource_metrics"),
	native("elasticsearch", "Elasticsearch", "database", "elasticsearch", "elasticsearchreceiver"),
	deferred("jvm", "JVM", "runtime", "external_jmx_process_required"),
	deferred("tomcat", "Tomcat", "runtime", "external_jmx_process_required"),
	deferred("springboot", "Spring Boot", "runtime", "sdk_push_is_not_dedicated_resource_collection"),
	deferred("minio", "MinIO", "storage", "no_dedicated_receiver"),
	native("http", "HTTP / HTTPS", "probe", "http_check", "httpcheckreceiver"),
	native("tcp", "TCP", "probe", "tcp_check", "tcpcheckreceiver"),
	native("tls", "TLS", "probe", "tls_check", "tlscheckreceiver"),
	native("dns", "DNS", "probe", "dns_check", "dnscheckreceiver"),
	native("icmp", "ICMP", "probe", "icmpcheckreceiver", "icmpcheckreceiver"),
	ingestion("otlp", "OpenTelemetry / OTLP", "otlp", "otlpreceiver", "metrics", "logs", "traces"),
	ingestion("skywalking", "SkyWalking", "skywalking", "skywalkingreceiver", "traces"),
	ingestion("jaeger", "Jaeger", "jaeger", "jaegerreceiver", "traces"),
	ingestion("filelog", "File logs", "filelog", "filelogreceiver", "logs"),
	ingestion("k8slog", "Kubernetes logs", "filelog", "filelogreceiver", "logs"),
	ingestion("sdklog", "SDK logs", "otlp", "otlpreceiver", "logs"),
	ingestion("journald", "Journald", "journald", "journaldreceiver", "logs"),
	ingestion("eventlog", "Windows Event Log", "windowseventlog", "windowseventlogreceiver", "logs"),
}
