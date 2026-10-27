/** Generated from fixed collection definitions. DO NOT EDIT. */
// prettier-ignore
export const builtinCollections = [
  {
    "key": "host_metrics",
    "name": "Host metrics",
    "resource_types": [
      "host"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "component": "hostmetrics",
    "source_type": "hostmetrics",
    "placement": "host",
    "legacy_profile": "host-basic",
    "component_tls_supported": false
  },
  {
    "key": "host_process_metrics",
    "name": "Host processes",
    "resource_types": [
      "host"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "component": "hostmetrics",
    "source_type": "host_processes",
    "placement": "host",
    "legacy_profile": "",
    "component_tls_supported": false
  },
  {
    "key": "host_systemd_metrics",
    "name": "Linux system services",
    "resource_types": [
      "host"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "systemd",
    "source_type": "host_systemd",
    "placement": "host",
    "legacy_profile": "",
    "component_tls_supported": false
  },
  {
    "key": "host_windows_service_metrics",
    "name": "Windows system services",
    "resource_types": [
      "host"
    ],
    "platforms": [
      "windows_amd64"
    ],
    "component": "windows_service",
    "source_type": "host_windows_services",
    "placement": "host",
    "legacy_profile": "",
    "component_tls_supported": false
  },
  {
    "key": "collector_metrics",
    "name": "Collector metrics",
    "resource_types": [
      "host",
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64",
      "windows_amd64"
    ],
    "component": "prometheus",
    "source_type": "collector_self",
    "placement": "all",
    "legacy_profile": "collector-self",
    "component_tls_supported": false
  },
  {
    "key": "kubernetes_cluster_metrics",
    "name": "Cluster and workload state",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "k8s_cluster",
    "source_type": "k8s_cluster",
    "placement": "kubernetes_gateway",
    "legacy_profile": "k8s-cluster",
    "component_tls_supported": false
  },
  {
    "key": "kubernetes_node_metrics",
    "name": "Node and container metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "kubeletstats",
    "source_type": "kubeletstats",
    "placement": "kubernetes_agent",
    "legacy_profile": "k8s-node-container",
    "component_tls_supported": false
  },
  {
    "key": "kubernetes_apiserver_metrics",
    "name": "API server metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "prometheus",
    "source_type": "kubernetes_apiserver",
    "placement": "kubernetes_gateway",
    "legacy_profile": "",
    "component_tls_supported": false
  },
  {
    "key": "kubernetes_coredns_metrics",
    "name": "CoreDNS metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "prometheus",
    "source_type": "kubernetes_coredns",
    "placement": "kubernetes_gateway",
    "legacy_profile": "",
    "component_tls_supported": false
  },
  {
    "key": "kubernetes_scheduler_metrics",
    "name": "Scheduler metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "prometheus",
    "source_type": "kubernetes_scheduler",
    "placement": "kubernetes_gateway",
    "legacy_profile": "",
    "component_tls_supported": true
  },
  {
    "key": "kubernetes_controller_metrics",
    "name": "Controller Manager metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "prometheus",
    "source_type": "kubernetes_controller_manager",
    "placement": "kubernetes_gateway",
    "legacy_profile": "",
    "component_tls_supported": true
  },
  {
    "key": "kubernetes_etcd_metrics",
    "name": "etcd component metrics",
    "resource_types": [
      "kubernetes_cluster"
    ],
    "platforms": [
      "linux_amd64",
      "linux_arm64"
    ],
    "component": "prometheus",
    "source_type": "kubernetes_etcd",
    "placement": "kubernetes_gateway",
    "legacy_profile": "",
    "component_tls_supported": false
  }
] as const;
