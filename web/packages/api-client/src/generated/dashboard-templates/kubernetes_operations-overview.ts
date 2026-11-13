/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "kubernetes.operations-overview",
  "version": 3,
  "name": "Kubernetes 运维大盘 / Operations overview",
  "description": "当前授权资源的状态、分布、关系和告警；历史趋势来自真实采集。Current authorized resource state and relationships, with real telemetry trends.",
  "integration_keys": [],
  "spec": {
    "schema_version": "argus.telemetry_dashboard/v2",
    "default_time_range": {
      "kind": "relative",
      "seconds": 3600
    },
    "default_refresh_seconds": 30,
    "variables": [],
    "panels": [
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "limit": 20
        },
        "id": "health",
        "title": "资源状态 / Resource status",
        "description": "",
        "type": "resource_summary",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 0,
          "w": 9,
          "h": 28,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "alert_instances",
          "limit": 20
        },
        "id": "active_alerts",
        "title": "活跃告警 / Active alerts",
        "description": "",
        "type": "alert_summary",
        "signal": "alerts",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "alert_instances",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 9,
          "y": 0,
          "w": 3,
          "h": 28,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "limit": 20
        },
        "id": "nodes",
        "title": "节点与 Pod 分布 / Nodes and Pods",
        "description": "",
        "type": "node_allocation",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 28,
          "w": 6,
          "h": 42,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "limit": 20
        },
        "id": "distribution",
        "title": "资源状态构成 / Resource distribution",
        "description": "",
        "type": "resource_distribution",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 28,
          "w": 6,
          "h": 42,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "id": "cpu",
        "title": "CPU 使用 / CPU usage",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubeletstats",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum by (resource_k8s_namespace_name) (k8s_pod_cpu_usage)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 300,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 70,
          "w": 6,
          "h": 52,
          "min_w": 4,
          "min_h": 24
        },
        "unit": "cores",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "memory",
        "title": "内存使用 / Memory usage",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubeletstats",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum by (resource_k8s_namespace_name) (k8s_pod_memory_usage)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 300,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 70,
          "w": 6,
          "h": 52,
          "min_w": 4,
          "min_h": 24
        },
        "unit": "bytes",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "cpu_requests",
        "title": "CPU 请求 / CPU requests",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "k8s_cluster",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum by (resource_k8s_namespace_name) (k8s_container_cpu_request)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 300,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 122,
          "w": 6,
          "h": 52,
          "min_w": 4,
          "min_h": 24
        },
        "unit": "cores",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "memory_requests",
        "title": "内存请求 / Memory requests",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "k8s_cluster",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum by (resource_k8s_namespace_name) (k8s_container_memory_request)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 300,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 122,
          "w": 6,
          "h": 52,
          "min_w": 4,
          "min_h": 24
        },
        "unit": "bytes",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "limit": 20
        },
        "id": "relations",
        "title": "资源关系 / Resource relationships",
        "description": "",
        "type": "resource_graph",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 174,
          "w": 12,
          "h": 62,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "resource_types": [
            "event"
          ],
          "states": [
            "warning"
          ],
          "limit": 20
        },
        "id": "events",
        "title": "近期警告事件 / Recent warning events",
        "description": "",
        "type": "resource_events",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 236,
          "w": 6,
          "h": 34,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "kubernetes_inventory",
          "resource_types": [
            "pod",
            "deployment",
            "statefulset",
            "daemonset",
            "job",
            "pvc"
          ],
          "states": [
            "failed",
            "warning",
            "pending",
            "unknown"
          ],
          "limit": 20
        },
        "id": "attention",
        "title": "需要关注的资源 / Resources needing attention",
        "description": "",
        "type": "resource_table",
        "signal": "resources",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "kubernetes_inventory",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 236,
          "w": 6,
          "h": 34,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      },
      {
        "data_source": {
          "kind": "alert_instances",
          "limit": 20
        },
        "id": "alerts",
        "title": "活跃告警 / Active alerts",
        "description": "",
        "type": "alert_list",
        "signal": "alerts",
        "authoring_mode": "builder",
        "applicable_resource_types": [
          "kubernetes_cluster",
          "kubernetes_namespace"
        ],
        "source_binding": {
          "source_type": "alert_instances",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 270,
          "w": 12,
          "h": 34,
          "min_w": 2,
          "min_h": 12
        },
        "unit": "",
        "decimals": 0,
        "legend": false,
        "thresholds": []
      }
    ],
    "layout": {
      "columns": 12,
      "row_height": 8
    }
  },
  "references": []
} as const;
