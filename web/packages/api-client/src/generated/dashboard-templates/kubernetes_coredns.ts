/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "kubernetes.coredns",
  "version": 1,
  "name": "Kubernetes CoreDNS",
  "description": "",
  "integration_keys": [
    "kubernetes_coredns_metrics"
  ],
  "spec": {
    "schema_version": "argus.telemetry_dashboard/v1",
    "default_time_range": {
      "kind": "relative",
      "seconds": 3600
    },
    "default_refresh_seconds": 30,
    "variables": [],
    "panels": [
      {
        "id": "dns_up",
        "title": "up",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "up"
              }
            },
            "query_mode": "instant",
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
          "y": 0,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "dns_requests",
        "title": "sum(rate(coredns_dns_requests_total[5m]))",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(rate(coredns_dns_requests_total[5m]))"
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
          "y": 0,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "req/s",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "dns_responses",
        "title": "sum(rate(coredns_dns_responses_total[5m]))",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(rate(coredns_dns_responses_total[5m]))"
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
          "y": 36,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "req/s",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "dns_p95",
        "title": "histogram_quantile(0.95, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket[5m])))",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "histogram_quantile(0.95, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket[5m])))"
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
          "y": 36,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "s",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "dns_cache_hits",
        "title": "sum(rate(coredns_cache_hits_total[5m])) / sum(rate(coredns_cache_requests_total[5m]))",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(rate(coredns_cache_hits_total[5m])) / sum(rate(coredns_cache_requests_total[5m]))"
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
          "y": 72,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "percent_ratio",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "dns_memory",
        "title": "process_resident_memory_bytes",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "kubernetes_cluster"
        ],
        "source_binding": {
          "source_type": "kubernetes_coredns",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "process_resident_memory_bytes"
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
          "y": 72,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "bytes",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      }
    ],
    "layout": {
      "columns": 12,
      "row_height": 8
    }
  },
  "references": [
    {
      "title": "Kubernetes CoreDNS Grafana reference",
      "url": "https://grafana.com/grafana/dashboards/15762-kubernetes-system-coredns/",
      "grafana_dashboard_id": "15762"
    }
  ]
} as const;
