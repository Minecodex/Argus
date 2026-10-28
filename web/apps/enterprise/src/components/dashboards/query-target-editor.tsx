import { useEnterpriseAuthStore } from "@argus/auth";
import { useQueryCatalog } from "./query-scope";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  type DashboardPanel,
  type DashboardSpec,
  type DashboardTarget,
  type DashboardSchemas,
} from "@argus/api-client";
import {
  Button,
  ComboBox,
  Field,
  Input,
  Select,
  TelemetryQueryEditor,
  TelemetryEditorProvider,
  useTelemetryEditorEnvironment,
  JsonField,
} from "@argus/ui";
import { FilterFields } from "./filter-fields";
import { ParameterBindingsEditor } from "./parameter-bindings-editor";
import { DashboardFieldExplorer } from "./field-explorer";
import { useDashboardQueryScope, queryScopeBounds } from "./query-scope";
import { useMetricMetadata } from "./use-metric-metadata";
import { useMetricOptions } from "./use-metric-options";
import { MetricQueryFields } from "./metric-query-fields";
import { MetricLabelFilters } from "./metric-label-filters";
import { MetricQueryOptions } from "./metric-query-options";
import {
  knownMetricType,
  metricOperationCompatible,
  requiredMetricType,
  selectMetric,
} from "./metric-metadata";
export function QueryTargetEditor({
  panel,
  spec,
  target,
  onChange,
  onMetadataChange,
  onInvalidInput,
  rowInputs = [],
  detail = false,
  compactMetrics: compactMetricsProp = false,
  metricLayout = false,
  transient = false,
  sourceControl,
}: {
  panel: DashboardPanel;
  spec: DashboardSpec;
  target: DashboardTarget;
  onChange: (target: DashboardTarget) => void;
  onMetadataChange?: (target: DashboardTarget) => void;
  onInvalidInput?: () => void;
  rowInputs?: string[];
  detail?: boolean;
  compactMetrics?: boolean;
  metricLayout?: boolean;
  transient?: boolean;
  sourceControl?: ReactNode;
}) {
  const { t } = useTranslation();
  const compactMetrics =
    compactMetricsProp || (metricLayout && panel.signal === "metrics");
  const session = useEnterpriseAuthStore((s) => s.session);
  const catalogApi = useQueryCatalog(),
    outerEditor = useTelemetryEditorEnvironment();
  const scope = useDashboardQueryScope(),
    time = scope?.time ?? spec.default_time_range;
  const editorKey = JSON.stringify([
    session?.session,
    scope?.key,
    scope?.resources,
    time,
    panel.source_binding,
    panel.signal,
  ]);
  const editor = useMemo(
    () =>
      outerEditor.key !== "offline"
        ? outerEditor
        : {
            key: editorKey,
            load:
              scope?.catalog === false
                ? undefined
                : async (
                    request: import("@argus/ui").TelemetryCompletionRequest,
                    signal: AbortSignal,
                  ) =>
                    catalogApi(
                      {
                        ...queryScopeBounds(time),
                        ...request,
                        signal: panel.signal,
                        source_binding: panel.source_binding,
                        resource_ids: scope?.resources ?? [],
                        filters: request.filters ?? [],
                        selected_values: [],
                        limit: 100,
                      },
                      signal,
                    ),
          },
    [
      outerEditor,
      editorKey,
      catalogApi,
      scope?.catalog,
      scope?.resources,
      time,
      panel.signal,
      panel.source_binding,
    ],
  );
  type Metric = DashboardSchemas["DashboardMetricDescriptor"];
  const [catalogState, writeCatalog] = useState<{
    key: string;
    items: Metric[];
  }>({ key: editorKey, items: [] });
  const catalog = catalogState.key === editorKey ? catalogState.items : [];
  const setCatalog = (value: Metric[] | ((previous: Metric[]) => Metric[])) =>
    writeCatalog((state) => ({
      key: editorKey,
      items:
        typeof value === "function"
          ? value(state.key === editorKey ? state.items : [])
          : value,
    }));
  const [error, setError] = useState("");
  const candidateRequest = useRef<AbortController | null>(null);
  useEffect(() => {
    candidateRequest.current?.abort();
    setError("");
    return () => candidateRequest.current?.abort();
  }, [editorKey]);
  const patchTarget = (patch: Partial<DashboardTarget>) =>
    onChange({ ...target, ...patch });
  const builder = target.source_definition.builder;
  const fetchMetrics = useMetricOptions(
    panel.source_binding,
    { time, resources: scope?.resources ?? [] },
    (metrics) => {
      setCatalog((previous) => {
        const all = new Map(previous.map((item) => [item.name, item]));
        for (const item of metrics) all.set(item.name, item);
        return [...all.values()];
      });
    },
  );
  const { metadata } = useMetricMetadata(
    panel,
    target,
    { time, resources: scope?.resources ?? [], catalog: scope?.catalog },
    onMetadataChange ?? onChange,
  );
  const patchBuilder = (patch: Partial<DashboardSchemas["DashboardBuilder"]>) =>
    patchTarget({ source_definition: { builder: { ...builder!, ...patch } } });
  const operations =
    panel.signal === "metrics"
      ? [
          "value",
          "sum",
          "avg",
          "min",
          "max",
          "rate",
          "error_rate",
          "topk",
          "p95",
        ]
      : panel.signal === "logs"
        ? [
            "records",
            "count",
            "count_by",
            "count_over_time",
            ...(detail ? ["log_context"] : []),
          ]
        : detail
          ? [
              "list",
              "detail",
              "trace_graph",
              "apm_services",
              "apm_instances",
              "apm_endpoints",
              "apm_red",
              "apm_topology",
            ]
          : panel.type.startsWith("apm_")
            ? [panel.type]
            : ["list", "detail"];
  const content = (
    <div className="argus-dashboard-form-stack">
      <TelemetryEditorProvider value={editor}>
        <TelemetryQueryEditor
          mode={panel.authoring_mode as "builder" | "dsl"}
          language={target.language}
          expression={target.source_definition.dsl?.expression}
          pipeline={target.source_definition.dsl?.pipeline}
          onExpressionChange={(expression) =>
            patchTarget({
              source_definition: {
                dsl: { ...target.source_definition.dsl!, expression },
              },
            })
          }
          onPipelineChange={(pipeline) =>
            patchTarget({
              source_definition: {
                dsl: { ...target.source_definition.dsl!, pipeline },
              },
            })
          }
          builder={
            builder && (
              <div className="argus-dashboard-form-stack">
                {!compactMetrics && (
                  <Field
                    label={t("dashboards.operation")}
                    requirement="required"
                  >
                    <Select
                      value={builder.operation}
                      options={operations.map((value) => ({
                        value,
                        label: t(`dashboards.operations.${value}`),
                        disabled:
                          panel.signal === "metrics" &&
                          !metricOperationCompatible(value, metadata),
                        description:
                          panel.signal === "metrics" &&
                          requiredMetricType(value)
                            ? t("dashboards.editor.metricRequirement", {
                                type: requiredMetricType(value),
                              })
                            : undefined,
                      }))}
                      onValueChange={(operation) =>
                        patchBuilder({
                          operation:
                            operation as DashboardSchemas["DashboardBuilder"]["operation"],
                          window_seconds: [
                            "rate",
                            "p95",
                            "error_rate",
                          ].includes(operation)
                            ? (builder.window_seconds ?? 300)
                            : undefined,
                          error_filters:
                            operation === "error_rate"
                              ? (builder.error_filters ?? [
                                  {
                                    field: "status",
                                    operator: "=~",
                                    value: "5..",
                                  },
                                ])
                              : undefined,
                          bucket_seconds: [
                            "count_over_time",
                            "apm_red",
                          ].includes(operation)
                            ? (builder.bucket_seconds ?? 60)
                            : undefined,
                          top_n:
                            operation === "topk"
                              ? (builder.top_n ?? 10)
                              : undefined,
                          context_before:
                            operation === "log_context"
                              ? (builder.context_before ?? 20)
                              : undefined,
                          context_after:
                            operation === "log_context"
                              ? (builder.context_after ?? 20)
                              : undefined,
                          trace_id:
                            operation === "detail"
                              ? builder.trace_id
                              : undefined,
                          limit:
                            (panel.signal === "logs" &&
                              operation !== "log_context") ||
                            operation === "list" ||
                            operation.startsWith("apm_")
                              ? builder.limit
                              : undefined,
                          group_by: [
                            "sum",
                            "avg",
                            "min",
                            "max",
                            "p95",
                            "error_rate",
                            "count_by",
                            "count_over_time",
                          ].includes(operation)
                            ? builder.group_by
                            : [],
                        })
                      }
                    />
                  </Field>
                )}
                {panel.signal === "metrics" && (
                  <>
                    {!compactMetrics && (
                      <div className="argus-dashboard-inline">
                        <Field
                          label={t("dashboards.metric")}
                          requirement="required"
                        >
                          <ComboBox
                            disabled={scope?.catalog === false}
                            value={builder.metric ?? ""}
                            allowCustom
                            options={catalog.map((metric) => ({
                              value: metric.name,
                              label: metric.name,
                              description: [metric.type, metric.unit]
                                .filter(Boolean)
                                .join(" · "),
                            }))}
                            contextKey={JSON.stringify([
                              panel.source_binding,
                              time,
                              scope?.resources,
                            ])}
                            fetchOptions={
                              scope?.catalog === false
                                ? undefined
                                : fetchMetrics
                            }
                            onValueChange={(metric) =>
                              patchBuilder(selectMetric(builder, metric))
                            }
                          />
                        </Field>
                        <Button
                          isDisabled={scope?.catalog === false}
                          onPress={() => {
                            candidateRequest.current?.abort();
                            const controller = new AbortController();
                            candidateRequest.current = controller;
                            void catalogApi(
                              {
                                ...queryScopeBounds(time),
                                resource_ids: scope?.resources ?? [],
                                selected_values: [],
                                filters: [],
                                signal: "metrics",
                                source_binding: panel.source_binding,
                                kind: "metrics",
                                limit: 100,
                              },
                              controller.signal,
                            )
                              .then((result) => {
                                if (!controller.signal.aborted)
                                  setCatalog(result.metrics);
                              })
                              .catch(() => {
                                if (!controller.signal.aborted)
                                  setError(t("dashboards.failed"));
                              });
                          }}
                        >
                          {t("dashboards.candidates")}
                        </Button>
                      </div>
                    )}
                    <Field
                      label={t("dashboards.metricType")}
                      requirement="optional"
                    >
                      <Select
                        value={builder.metric_type ?? ""}
                        disabled={!!knownMetricType(metadata)}
                        options={[
                          { value: "", label: "—" },
                          {
                            value: "gauge",
                            label: t("dashboards.metricGauge"),
                          },
                          {
                            value: "counter",
                            label: t("dashboards.metricCounter"),
                          },
                          {
                            value: "histogram",
                            label: t("dashboards.metricHistogram"),
                          },
                        ]}
                        onValueChange={(metric_type) =>
                          patchBuilder({
                            metric_type: metric_type || undefined,
                          })
                        }
                      />
                    </Field>
                  </>
                )}
                {!compactMetrics &&
                  [
                    "sum",
                    "avg",
                    "min",
                    "max",
                    "p95",
                    "error_rate",
                    "count_by",
                    "count_over_time",
                  ].includes(builder.operation) && (
                    <Field
                      label={t("dashboards.groupBy")}
                      requirement="optional"
                    >
                      <Input
                        value={builder.group_by.join(", ")}
                        onChange={(e) =>
                          patchBuilder({
                            group_by: e.target.value
                              .split(",")
                              .map((s) => s.trim())
                              .filter(Boolean),
                          })
                        }
                      />
                    </Field>
                  )}
                {["rate", "p95", "error_rate"].includes(builder.operation) && (
                  <Field label={t("dashboards.window")} requirement="required">
                    <Input
                      type="number"
                      min={1}
                      max={86400}
                      value={builder.window_seconds ?? 300}
                      onChange={(e) =>
                        patchBuilder({ window_seconds: Number(e.target.value) })
                      }
                    />
                  </Field>
                )}
                {["count_over_time", "apm_red"].includes(builder.operation) && (
                  <Field label={t("dashboards.bucket")} requirement="required">
                    <Input
                      type="number"
                      min={1}
                      max={86400}
                      value={builder.bucket_seconds ?? 60}
                      onChange={(e) =>
                        patchBuilder({ bucket_seconds: Number(e.target.value) })
                      }
                    />
                  </Field>
                )}
                {builder.operation === "detail" && (
                  <Field label={t("dashboards.traceId")} requirement="required">
                    <Input
                      value={builder.trace_id ?? ""}
                      onChange={(e) =>
                        patchBuilder({ trace_id: e.target.value })
                      }
                    />
                  </Field>
                )}
                {builder.operation === "topk" && (
                  <Field
                    label={t("dashboards.operations.topk")}
                    requirement="required"
                  >
                    <Input
                      type="number"
                      min={1}
                      max={100}
                      value={builder.top_n ?? 10}
                      onChange={(e) =>
                        patchBuilder({ top_n: Number(e.target.value) })
                      }
                    />
                  </Field>
                )}
                {panel.signal !== "metrics" &&
                  !["detail", "trace_graph", "log_context"].includes(
                    builder.operation,
                  ) && (
                    <Field label={t("dashboards.limit")} requirement="optional">
                      <Input
                        type="number"
                        min={1}
                        max={10000}
                        value={builder.limit ?? ""}
                        onChange={(e) =>
                          patchBuilder({
                            limit: e.target.value
                              ? Number(e.target.value)
                              : undefined,
                          })
                        }
                      />
                    </Field>
                  )}
                {!compactMetrics && (
                  <FilterFields
                    onInvalidInput={onInvalidInput}
                    filters={builder.filters}
                    onChange={(filters) => patchBuilder({ filters })}
                    variables={spec.variables}
                    rowInputs={rowInputs}
                    locals={panel.local_filters.map((f) => f.id)}
                  />
                )}
                {builder.operation === "error_rate" && (
                  <section aria-label={t("dashboards.errorFilters")}>
                    <strong>{t("dashboards.errorFilters")}</strong>
                    <p>{t("dashboards.errorRateHint")}</p>
                    <FilterFields
                      onInvalidInput={onInvalidInput}
                      filters={builder.error_filters ?? []}
                      onChange={(error_filters) =>
                        patchBuilder({ error_filters })
                      }
                      variables={[]}
                      operators={["=", "!=", "=~", "!~"]}
                    />
                  </section>
                )}
              </div>
            )
          }
        />
      </TelemetryEditorProvider>
      {scope?.catalog !== false && (
        <details>
          <summary>{t("dashboards.fieldExplorer")}</summary>
          <DashboardFieldExplorer
            key={JSON.stringify([
              scope?.key,
              scope?.resources,
              panel.source_binding,
              panel.signal,
              builder?.metric,
              spec.default_time_range,
            ])}
            signal={panel.signal as "metrics" | "logs" | "traces"}
            source={panel.source_binding}
            time={spec.default_time_range}
            metric={builder?.metric}
            canUse={(field) =>
              panel.signal !== "traces" ||
              field.startsWith("attributes.") ||
              field.startsWith("resource_attributes.") ||
              [
                "service_name",
                "operation",
                ...(builder?.operation === "list"
                  ? ["status", "trace_id"]
                  : []),
              ].includes(field)
            }
            onUse={
              builder
                ? (field, value) => {
                    const names: Record<string, string> = {
                      service_name: "serviceName",
                      operation: "operationName",
                      trace_id: "traceId",
                    };
                    const name =
                      panel.signal === "traces"
                        ? (names[field] ?? field)
                        : field;
                    patchBuilder({
                      filters: [
                        ...builder.filters.filter(
                          (f) => f.field !== name || f.operator !== "=",
                        ),
                        { field: name, operator: "=", value },
                      ],
                    });
                  }
                : undefined
            }
          />
        </details>
      )}
      {!transient && (
        <ParameterBindingsEditor
          value={target.parameter_bindings}
          onChange={(parameter_bindings) => patchTarget({ parameter_bindings })}
          variables={spec.variables}
          rowInputs={rowInputs}
          locals={panel.local_filters.map((f) => f.id)}
        />
      )}
      <details
        open={transient ? undefined : true}
        className="argus-query-advanced"
      >
        <summary>{t("telemetry.queryOptions")}</summary>
        {panel.signal === "metrics" && (
          <div className="argus-dashboard-form-grid">
            <Field label={t("dashboards.queryMode")} requirement="required">
              <Select
                value={target.query_mode}
                onValueChange={(query_mode) => {
                  if (query_mode === "range" || query_mode === "instant")
                    patchTarget({ query_mode });
                }}
                options={[
                  { value: "range", label: t("dashboards.range") },
                  { value: "instant", label: t("dashboards.instant") },
                ]}
              />
            </Field>
            {target.query_mode === "range" && (
              <>
                <Field
                  label={t("dashboards.stepPolicy")}
                  requirement="required"
                >
                  <Select
                    value={target.range_step_policy.kind}
                    options={[
                      { value: "auto", label: t("dashboards.autoStep") },
                      { value: "fixed", label: t("dashboards.fixedStep") },
                    ]}
                    onValueChange={(kind) =>
                      patchTarget({
                        range_step_policy:
                          kind === "fixed"
                            ? { kind, seconds: 60 }
                            : {
                                kind: "auto",
                                target_points: 300,
                                min_step_seconds: 1,
                              },
                      })
                    }
                  />
                </Field>
                <Field
                  label={t(
                    target.range_step_policy.kind === "fixed"
                      ? "dashboards.stepSeconds"
                      : "dashboards.targetPoints",
                  )}
                  requirement="required"
                >
                  <Input
                    type="number"
                    min={1}
                    value={
                      target.range_step_policy.kind === "fixed"
                        ? target.range_step_policy.seconds
                        : target.range_step_policy.target_points
                    }
                    onChange={(e) =>
                      patchTarget({
                        range_step_policy: {
                          ...target.range_step_policy,
                          ...(target.range_step_policy.kind === "fixed"
                            ? { seconds: Number(e.target.value) }
                            : { target_points: Number(e.target.value) }),
                        },
                      })
                    }
                  />
                </Field>
              </>
            )}
          </div>
        )}

        {builder?.operation === "log_context" && (
          <div className="argus-dashboard-form-grid">
            {["context_before", "context_after"].map((key) => (
              <Field
                key={key}
                label={t(
                  key === "context_before"
                    ? "dashboards.contextBefore"
                    : "dashboards.contextAfter",
                )}
                requirement="required"
              >
                <Input
                  type="number"
                  min={0}
                  max={200}
                  value={builder[key as "context_before"] ?? 20}
                  onChange={(e) =>
                    patchBuilder({ [key]: Number(e.target.value) })
                  }
                />
              </Field>
            ))}
          </div>
        )}
        {target.source_definition.dsl &&
          target.language === "skywalking_graphql" && (
            <>
              <Field
                label={t("dashboards.operationName")}
                requirement="optional"
              >
                <Input
                  value={target.source_definition.dsl.operation ?? ""}
                  onChange={(e) =>
                    patchTarget({
                      source_definition: {
                        dsl: {
                          ...target.source_definition.dsl!,
                          operation: e.target.value || undefined,
                        },
                      },
                    })
                  }
                />
              </Field>
              <JsonField
                onInvalidChange={(invalid) => {
                  if (invalid) onInvalidInput?.();
                }}
                label={t("dashboards.graphVariables")}
                kind="object"
                value={target.source_definition.dsl.variables ?? {}}
                onChange={(variables) =>
                  patchTarget({
                    source_definition: {
                      dsl: { ...target.source_definition.dsl!, variables },
                    },
                  })
                }
              />
            </>
          )}
      </details>
      {error && <p role="alert">{error}</p>}
    </div>
  );
  return metricLayout && panel.signal === "metrics" ? (
    <div className="argus-dashboard-form-stack">
      {builder ? (
        <>
          <MetricQueryFields
            builder={builder}
            onChange={patchBuilder}
            metadata={metadata}
            metrics={catalog}
            contextKey={editorKey}
            fetchMetrics={fetchMetrics}
            disabled={scope?.catalog === false}
            sourceControl={sourceControl}
          />
          <MetricLabelFilters
            panel={panel}
            spec={spec}
            target={target}
            filters={builder.filters}
            labels={metadata?.labels ?? []}
            onInvalidInput={onInvalidInput}
            onChange={(filters) => patchBuilder({ filters })}
          />
          {builder.operation === "error_rate" && (
            <section aria-label={t("dashboards.errorFilters")}>
              <strong>{t("dashboards.errorFilters")}</strong>
              <MetricLabelFilters
                panel={panel}
                spec={spec}
                target={target}
                filters={builder.error_filters ?? []}
                labels={metadata?.labels ?? []}
                literalOnly
                onInvalidInput={onInvalidInput}
                onChange={(error_filters) => patchBuilder({ error_filters })}
              />
            </section>
          )}
        </>
      ) : (
        <>
          {sourceControl}
          <TelemetryEditorProvider value={editor}>
            <TelemetryQueryEditor
              mode="dsl"
              language={target.language}
              expression={target.source_definition.dsl?.expression}
              onExpressionChange={(expression) =>
                patchTarget({
                  source_definition: {
                    dsl: { ...target.source_definition.dsl!, expression },
                  },
                })
              }
            />
          </TelemetryEditorProvider>
        </>
      )}
      <MetricQueryOptions target={target} onChange={patchTarget} />
      {!transient &&
        (spec.variables.length > 0 ||
          panel.local_filters.length > 0 ||
          target.parameter_bindings.length > 0) && (
          <ParameterBindingsEditor
            value={target.parameter_bindings}
            onChange={(parameter_bindings) =>
              patchTarget({ parameter_bindings })
            }
            variables={spec.variables}
            locals={panel.local_filters.map((f) => f.id)}
            rowInputs={rowInputs}
          />
        )}
    </div>
  ) : (
    <div className="argus-dashboard-form-stack">
      {sourceControl}
      {content}
    </div>
  );
}
