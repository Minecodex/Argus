/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "host.overview",
  "version": 1,
  "name": "Host",
  "description": "",
  "integration_keys": [
    "host_metrics"
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
        "id": "cpu",
        "title": "1 - avg by (argus_resource_id) (system_cpu_utilization{state=\"idle\"})",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "1 - avg by (argus_resource_id) (system_cpu_utilization{state=\"idle\"})"
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
          "y": 0,
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
        "id": "memory",
        "title": "system_memory_utilization{state=\"used\"}",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "system_memory_utilization{state=\"used\"}"
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
        "unit": "percent_ratio",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "memory_bytes",
        "title": "system_memory_usage{state=\"used\"}",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "system_memory_usage{state=\"used\"}"
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
          "y": 36,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "bytes",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "filesystem",
        "title": "system_filesystem_usage{state=\"used\"}",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "system_filesystem_usage{state=\"used\"}"
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
        "unit": "bytes",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "disk_io",
        "title": "rate(system_disk_io[5m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(system_disk_io[5m])"
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
        "unit": "B/s",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "network_io",
        "title": "rate(system_network_io[5m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "hostmetrics",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(system_network_io[5m])"
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
        "unit": "B/s",
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
      "title": "Host Grafana reference",
      "url": "https://grafana.com/grafana/dashboards/1860-node-exporter-full/",
      "grafana_dashboard_id": "1860"
    }
  ]
} as const;
