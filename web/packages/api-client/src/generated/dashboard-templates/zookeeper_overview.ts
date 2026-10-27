/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "zookeeper.overview",
  "version": 1,
  "name": "ZooKeeper",
  "description": "",
  "integration_keys": [
    "zookeeper"
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
        "id": "connections",
        "title": "zookeeper_connection_active",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "zookeeper_connection_active"
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
        "unit": "",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "znodes",
        "title": "zookeeper_znode_count",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "zookeeper_znode_count"
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
        "id": "watches",
        "title": "zookeeper_watch_count",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "zookeeper_watch_count"
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
        "unit": "",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "requests",
        "title": "zookeeper_request_active",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "zookeeper_request_active"
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
      },
      {
        "id": "latency",
        "title": "zookeeper_latency_avg",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "zookeeper_latency_avg"
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
        "unit": "ms",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "packets",
        "title": "rate(zookeeper_packet_count[5m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "zookeeper",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(zookeeper_packet_count[5m])"
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
        "unit": "ops/s",
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
      "title": "ZooKeeper Grafana reference",
      "url": "https://grafana.com/grafana/dashboards/10465-zookeeper/",
      "grafana_dashboard_id": "10465"
    }
  ]
} as const;
