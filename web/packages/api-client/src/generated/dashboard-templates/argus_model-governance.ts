/** Generated from locked dashboard template definitions. DO NOT EDIT. */
// prettier-ignore
export const template = {
  "key": "argus.model-governance",
  "version": 1,
  "name": "模型治理",
  "description": "内部模型调用、用量、结算金额与当前额度；自然月统计，按企业和部门授权。",
  "integration_keys": [],
  "spec": {
    "schema_version": "argus.telemetry_dashboard/v1",
    "default_time_range": {
      "kind": "relative",
      "seconds": 3600
    },
    "default_refresh_seconds": 0,
    "variables": [
      {
        "id": "model",
        "name": "model",
        "label": "模型",
        "multiple": true,
        "include_all": true,
        "default": {
          "all": true,
          "values": []
        },
        "query": {
          "signal": "metrics",
          "source_binding": {
            "source_type": "argus_internal_model_governance",
            "capability_version": "v1"
          },
          "field": "model_id",
          "filters": []
        }
      },
      {
        "id": "month",
        "name": "month",
        "label": "月份",
        "multiple": false,
        "include_all": false,
        "default": {
          "all": false,
          "values": []
        },
        "query": {
          "signal": "metrics",
          "source_binding": {
            "source_type": "argus_internal_model_governance",
            "capability_version": "v1"
          },
          "field": "month",
          "filters": []
        }
      }
    ],
    "panels": [
      {
        "id": "requests",
        "title": "调用数",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_requests)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
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
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "tokens",
        "title": "实报 Token",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_input_tokens) + sum(argus_model_output_tokens)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 4,
          "y": 0,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "cached",
        "title": "缓存输入 Token",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_cached_tokens)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 8,
          "y": 0,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "amount",
        "title": "结算金额",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_amount)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 20,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 6,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "success",
        "title": "成功率",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_successes) / sum(argus_model_requests)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 4,
          "y": 20,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "percent_ratio",
        "decimals": 2,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "latency",
        "title": "平均调用延迟",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_latency_ms) / sum(argus_model_latency_count)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 8,
          "y": 20,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "ms",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "errors",
        "title": "失败调用",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_errors)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 40,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "cancelled",
        "title": "取消调用",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_cancelled)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 4,
          "y": 40,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "compaction",
        "title": "压缩调用",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_compactions)"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 8,
          "y": 40,
          "w": 4,
          "h": 20,
          "min_w": 2,
          "min_h": 16
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "requests_trend",
        "title": "每日调用数",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_requests)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 60,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": [],
        "display": {
          "draw_style": "bar"
        }
      },
      {
        "id": "tokens_trend",
        "title": "每日实报 Token",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_input_tokens) + sum(argus_model_output_tokens)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 60,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 0,
        "legend": true,
        "thresholds": [],
        "display": {
          "draw_style": "bar"
        }
      },
      {
        "id": "amount_trend",
        "title": "每日结算金额",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "sum(argus_model_amount)"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 96,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 6,
        "legend": true,
        "thresholds": [],
        "display": {
          "draw_style": "bar"
        }
      },
      {
        "id": "p95",
        "title": "每日 P95 延迟",
        "description": "",
        "type": "timeseries",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "argus_model_latency_p95_ms"
              }
            },
            "query_mode": "range",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 96,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "ms",
        "decimals": 0,
        "legend": true,
        "thresholds": [],
        "display": {
          "draw_style": "bar"
        }
      },
      {
        "id": "models",
        "title": "模型金额排行",
        "description": "",
        "type": "bar",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "topk(20, sum by (model_id, model) (argus_model_amount))"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 132,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 6,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "departments",
        "title": "部门金额排行",
        "description": "",
        "type": "bar",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "topk(20, sum by (department_id, department) (argus_model_amount))"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 132,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 6,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "users",
        "title": "用户金额排行",
        "description": "",
        "type": "table",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "topk(50, sum by (user_id, user) (argus_model_amount))"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 168,
          "w": 6,
          "h": 36,
          "min_w": 2,
          "min_h": 20
        },
        "unit": "",
        "decimals": 6,
        "legend": true,
        "thresholds": []
      },
      {
        "id": "quota",
        "title": "所选月份额度占用（当前限额）",
        "description": "",
        "type": "bar_gauge",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "argus_model_quota_used / argus_model_quota_limit"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 168,
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
        "id": "tool_failures",
        "title": "工具调用失败",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "argus_model_tool_failures"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 0,
          "y": 204,
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
        "id": "structured_failures",
        "title": "结构化输出失败",
        "description": "",
        "type": "stat",
        "signal": "metrics",
        "authoring_mode": "dsl",
        "applicable_resource_types": [
          "internal_model"
        ],
        "source_binding": {
          "source_type": "argus_internal_model_governance",
          "capability_version": "v1"
        },
        "local_filters": [],
        "targets": [
          {
            "id": "a",
            "language": "promql",
            "source_definition": {
              "dsl": {
                "expression": "argus_model_structured_failures"
              }
            },
            "query_mode": "instant",
            "range_step_policy": {
              "kind": "auto",
              "target_points": 31,
              "min_step_seconds": 1
            },
            "parameter_bindings": []
          }
        ],
        "detail_query_targets": [],
        "drilldowns": [],
        "layout": {
          "x": 6,
          "y": 204,
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
  "references": []
} as const;
