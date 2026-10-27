/** Generated from internal/monitoringcatalog. DO NOT EDIT. */
export const monitoringIntegrations = [
  {
    "key": "mysql",
    "name": "MySQL",
    "family": "database",
    "component": "mysql",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mysqlreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "required",
      "database": true,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": true,
      "tls_server_name": true
    }
  },
  {
    "key": "postgresql",
    "name": "PostgreSQL",
    "family": "database",
    "component": "postgresql",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/postgresqlreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "required",
      "database": true,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": true,
      "tls_server_name": false
    }
  },
  {
    "key": "sqlserver",
    "name": "SQL Server",
    "family": "database",
    "component": "sqlserver",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sqlserverreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "required",
      "database": true,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": true,
      "tls_server_name": false
    }
  },
  {
    "key": "oracle",
    "name": "Oracle",
    "family": "database",
    "component": "oracledb",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/oracledbreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "required",
      "database": true,
      "database_required": true,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "mongodb",
    "name": "MongoDB",
    "family": "database",
    "component": "mongodb",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": true,
      "hostnames": false,
      "tls": true,
      "tls_server_name": true
    }
  },
  {
    "key": "clickhouse",
    "name": "ClickHouse",
    "family": "database",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "no_dedicated_receiver",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "tidb",
    "name": "TiDB",
    "family": "database",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "mysql_compatibility_is_not_tidb_collection",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "oceanbase",
    "name": "OceanBase",
    "family": "database",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "no_dedicated_receiver",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "redis",
    "name": "Redis",
    "family": "cache",
    "component": "redis",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/redisreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": true,
      "tls_server_name": true
    }
  },
  {
    "key": "memcached",
    "name": "Memcached",
    "family": "cache",
    "component": "memcached",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/memcachedreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": true,
      "tls_server_name": true
    }
  },
  {
    "key": "kafka",
    "name": "Kafka",
    "family": "messaging",
    "component": "kafka_metrics",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkametricsreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": true,
      "hostnames": false,
      "tls": true,
      "tls_server_name": true
    }
  },
  {
    "key": "rabbitmq",
    "name": "RabbitMQ",
    "family": "messaging",
    "component": "rabbitmq",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/rabbitmqreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "required",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "rocketmq",
    "name": "RocketMQ",
    "family": "messaging",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "no_dedicated_receiver",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "pulsar",
    "name": "Pulsar",
    "family": "messaging",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "receiver_transports_telemetry_not_broker_metrics",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "nats",
    "name": "NATS",
    "family": "messaging",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "no_eligible_mature_embedded_receiver",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "zookeeper",
    "name": "ZooKeeper",
    "family": "messaging",
    "component": "zookeeper",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zookeeperreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "nginx",
    "name": "Nginx",
    "family": "web",
    "component": "nginx",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/nginxreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "apache",
    "name": "Apache HTTP Server",
    "family": "web",
    "component": "apache",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/apachereceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "iis",
    "name": "IIS",
    "family": "web",
    "component": "iis",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/iisreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "local",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "haproxy",
    "name": "HAProxy",
    "family": "web",
    "component": "haproxy",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/haproxyreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "envoy",
    "name": "Envoy",
    "family": "web",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "access_log_receiver_is_not_resource_metrics",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "elasticsearch",
    "name": "Elasticsearch",
    "family": "database",
    "component": "elasticsearch",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/elasticsearchreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "jvm",
    "name": "JVM",
    "family": "runtime",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "external_jmx_process_required",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "tomcat",
    "name": "Tomcat",
    "family": "runtime",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "external_jmx_process_required",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "springboot",
    "name": "Spring Boot",
    "family": "runtime",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "sdk_push_is_not_dedicated_resource_collection",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "minio",
    "name": "MinIO",
    "family": "storage",
    "signals": [
      "metrics"
    ],
    "platforms": [],
    "selection": "deferred_no_eligible_embedded_component",
    "validation": "not_applicable",
    "reason": "no_dedicated_receiver",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "http",
    "name": "HTTP / HTTPS",
    "family": "probe",
    "component": "http_check",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/httpcheckreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "http_url",
      "authentication": "optional",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": true
    }
  },
  {
    "key": "tcp",
    "name": "TCP",
    "family": "probe",
    "component": "tcp_check",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/tcpcheckreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "tls",
    "name": "TLS",
    "family": "probe",
    "component": "tls_check",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/tlscheckreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "dns",
    "name": "DNS",
    "family": "probe",
    "component": "dns_check",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/dnscheckreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host_port",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": true,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "icmp",
    "name": "ICMP",
    "family": "probe",
    "component": "icmpcheckreceiver",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/icmpcheckreceiver",
    "signals": [
      "metrics"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true,
    "form": {
      "endpoint_kind": "host",
      "authentication": "none",
      "database": false,
      "database_required": false,
      "members": false,
      "hostnames": false,
      "tls": false,
      "tls_server_name": false
    }
  },
  {
    "key": "otlp",
    "name": "OpenTelemetry / OTLP",
    "family": "ingestion",
    "component": "otlp",
    "module": "go.opentelemetry.io/collector/receiver/otlpreceiver",
    "signals": [
      "metrics",
      "logs",
      "traces"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "skywalking",
    "name": "SkyWalking",
    "family": "ingestion",
    "component": "skywalking",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/skywalkingreceiver",
    "signals": [
      "traces"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "deferred_upstream_defect",
    "validation": "blocked",
    "reason": "upstream_acknowledgement_and_payload_logging",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "jaeger",
    "name": "Jaeger",
    "family": "ingestion",
    "component": "jaeger",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver",
    "signals": [
      "traces"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "filelog",
    "name": "File logs",
    "family": "ingestion",
    "component": "filelog",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver",
    "signals": [
      "logs"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true
  },
  {
    "key": "k8slog",
    "name": "Kubernetes logs",
    "family": "ingestion",
    "component": "filelog",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver",
    "signals": [
      "logs"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true
  },
  {
    "key": "sdklog",
    "name": "SDK logs",
    "family": "ingestion",
    "component": "otlp",
    "module": "go.opentelemetry.io/collector/receiver/otlpreceiver",
    "signals": [
      "logs"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  },
  {
    "key": "journald",
    "name": "Journald",
    "family": "ingestion",
    "component": "journald",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver",
    "signals": [
      "logs"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": true
  },
  {
    "key": "eventlog",
    "name": "Windows Event Log",
    "family": "ingestion",
    "component": "windowseventlog",
    "module": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowseventlogreceiver",
    "signals": [
      "logs"
    ],
    "platforms": [
      "windows_amd64"
    ],
    "selection": "candidate",
    "validation": "not_run",
    "config_version": "argus.monitoring_catalog/v1",
    "adapter_ready": false
  }
] as const;
