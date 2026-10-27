/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "host.systemd",
  "version": 2,
  "name": "Linux system services",
  "description": "",
  "integration_keys": [
    "host_systemd_metrics"
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
        "id": "active",
        "title": "sum(systemd_unit_state{systemd_unit_active_state=\"active\"})",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "host_systemd",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(systemd_unit_state{systemd_unit_active_state=\"active\"})"
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
        "id": "failed",
        "title": "sum(systemd_unit_state{systemd_unit_active_state=\"failed\"})",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "host_systemd",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(systemd_unit_state{systemd_unit_active_state=\"failed\"})"
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
          "x": 6,
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
        "id": "cpu",
        "title": "sum by (resource_systemd_unit_name) (rate(systemd_service_cpu_time[5m])) / 1000000",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "host_systemd",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum by (resource_systemd_unit_name) (rate(systemd_service_cpu_time[5m])) / 1000000"
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
        "unit": "cores",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "restarts",
        "title": "systemd_service_restarts",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "host"
        ],
        "source_binding": {
          "source_type": "host_systemd",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "systemd_service_restarts"
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
        "unit": "",
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
      "title": "Linux system services Grafana reference",
      "url": "https://grafana.com/grafana/dashboards/1860-node-exporter-full/",
      "grafana_dashboard_id": "1860"
    }
  ]
} as const;
