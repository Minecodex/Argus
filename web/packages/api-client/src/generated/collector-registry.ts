/** Generated from internal/otelcol/configbundle. DO NOT EDIT. */
export const collectorDistributionVersion = "0.1.0-m7";
export const collectorVersion = "0.133.0";
export const collectorConfigSchemaVersion = "argus.collector_config/v1";
export const collectorComponents = {
  "linux_amd64": [
    "argus_gateway_identity",
    "argus_identity",
    "batch",
    "file_storage",
    "filelog",
    "health_check",
    "hostmetrics",
    "jaeger",
    "journald",
    "k8s_cluster",
    "kubeletstats",
    "memory_limiter",
    "otlp",
    "prometheus",
    "resource",
    "skywalking"
  ],
  "linux_arm64": [
    "argus_gateway_identity",
    "argus_identity",
    "batch",
    "file_storage",
    "filelog",
    "health_check",
    "hostmetrics",
    "jaeger",
    "journald",
    "k8s_cluster",
    "kubeletstats",
    "memory_limiter",
    "otlp",
    "prometheus",
    "resource",
    "skywalking"
  ],
  "windows_amd64": [
    "argus_identity",
    "batch",
    "file_storage",
    "filelog",
    "health_check",
    "hostmetrics",
    "jaeger",
    "memory_limiter",
    "otlp",
    "prometheus",
    "resource",
    "skywalking",
    "windowseventlog"
  ]
} as const;
