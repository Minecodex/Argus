/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "sqlserver.overview",
  "version": 2,
  "name": "SQL Server",
  "description": "",
  "integration_keys": [
    "sqlserver"
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
        "title": "sqlserver_user_connection_count",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sqlserver_user_connection_count"
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
        "id": "batches",
        "title": "rate(sqlserver_batch_request_rate[1m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(sqlserver_batch_request_rate[1m])"
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
        "id": "cache",
        "title": "sqlserver_page_buffer_cache_hit_ratio",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sqlserver_page_buffer_cache_hit_ratio"
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
        "unit": "%",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "page_life",
        "title": "sqlserver_page_life_expectancy{performance_counter_object_name=~\".*Buffer Manager\"}",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sqlserver_page_life_expectancy{performance_counter_object_name=~\".*Buffer Manager\"}"
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
        "id": "lock_wait",
        "title": "rate(sqlserver_lock_wait_rate[1m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(sqlserver_lock_wait_rate[1m])"
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
        "unit": "req/s",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "compilations",
        "title": "rate(sqlserver_batch_sql_compilation_rate[1m])",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "monitoring_target"
        ],
        "source_binding": {
          "source_type": "sqlserver",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "rate(sqlserver_batch_sql_compilation_rate[1m])"
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
      "title": "SQL Server Grafana reference",
      "url": "https://grafana.com/grafana/dashboards/1883-telegraf-sql-server-monitoring/",
      "grafana_dashboard_id": "1883"
    }
  ]
} as const;
