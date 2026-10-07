import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  panelPreviewIdentity,
  dashboardPreviewSpec,
  type DashboardExecution,
  type DashboardPanel,
  type DashboardSchemas,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  Checkbox,
  ChartTypePicker,
  ComboBox,
  ConfirmDialog,
  Field,
  Input,
  SegmentedControl,
  ObservationPanel,
  observationChartCapabilities,
  PageShell,
  Select,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  ValueSelector,
} from "@argus/ui";
import { ArrowLeft, Play, Table2 } from "lucide-react";
import { useDashboardDraftWorkspace } from "../components/dashboards/draft-workspace-context";
import { QueryTargetEditor } from "../components/dashboards/query-target-editor";
import { QueryFilterChips } from "../components/dashboards/query-filter-chips";
import { LocalFiltersEditor } from "../components/dashboards/local-filters-editor";
import { DrilldownsEditor } from "../components/dashboards/drilldowns-editor";
import {
  DisplayEditor,
  compatibleDisplay,
} from "../components/dashboards/display-editor";
import { DrilldownViewer } from "../components/dashboards/drilldown-viewer";
import { TimeRangePicker } from "../components/dashboards/time-range-picker";
import {
  metricCharts,
  traceCharts,
  newTarget,
  sources,
  isDashboardSignal,
} from "../components/dashboards/model";
import { usePermission } from "../lib/permissions";
import { DashboardQueryScopeProvider } from "../components/dashboards/query-scope";
import {
  candidateContext,
  candidateRequest,
} from "../components/dashboards/candidate-filters";
import { useMetricMetadata } from "../components/dashboards/use-metric-metadata";
import {
  metricOperationCompatible,
  requiredMetricType,
  selectMetric,
} from "../components/dashboards/metric-metadata";
import "../styles/dashboard-panel-editor.css";

type Range = DashboardSchemas["DashboardTimeRange"];
type Parameters = DashboardSchemas["DashboardExecutionInput"];
function bounds(range: Range): { from: string; to: string } {
  if (range.kind === "absolute") return { from: range.from!, to: range.to! };
  const to = new Date();
  return {
    from: new Date(to.getTime() - (range.seconds ?? 3600) * 1000).toISOString(),
    to: to.toISOString(),
  };
}
export function DashboardPanelEditorPage() {
  const { panelId, draftId } = useParams({
    from: "/authed/admin/dashboard-drafts/$draftId/panels/$panelId",
  });
  const api = useApi(),
    editor = useDashboardDraftWorkspace(),
    { t } = useTranslation(),
    navigate = useNavigate(),
    canManage = usePermission("telemetry.dashboard.manage");
  const draft = editor.draft,
    panel = draft?.spec.panels.find((p) => p.id === panelId);
  const [tab, setTab] = useState("chart"),
    [resultView, setResultView] = useState<"chart" | "table">("chart"),
    [queryId, setQueryId] = useState(""),
    [range, setRange] = useState<Range>(),
    [resources, setResources] = useState<{ all: boolean; values: string[] }>({
      all: true,
      values: [],
    });
  const [variables, setVariables] = useState<
      NonNullable<Parameters["variables"]>
    >({}),
    [locals, setLocals] = useState<NonNullable<Parameters["local_values"]>>({});
  const [execution, setExecution] = useState<DashboardExecution>(),
    [lastIdentity, setLastIdentity] = useState(""),
    [status, setStatus] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [converting, setConverting] = useState(false),
    [generated, setGenerated] = useState(false);
  const [replace, setReplace] = useState<DashboardPanel["signal"]>(),
    [drill, setDrill] = useState<{
      row: Record<string, unknown>;
      target: string;
      token: string;
    }>();
  const request = useRef<AbortController | null>(null),
    sequence = useRef(0);
  const hosts = useQuery({
    queryKey: ["hosts", "dashboard-selector"],
    queryFn: () => api.hosts.list(),
    enabled: usePermission("host.read"),
  });
  const clusters = useQuery({
    queryKey: ["clusters", "dashboard-selector"],
    queryFn: () => api.kubernetes.listClusters(),
    enabled: usePermission("kubernetes.read"),
  });
  const parameters: Parameters = {
    panel_ids: [panelId],
    resource_ids: resources.all ? [] : resources.values,
    variables: {
      ...Object.fromEntries(
        (draft?.spec.variables ?? []).map((v) => [v.name, v.default]),
      ),
      ...variables,
    },
    local_values: {
      [panelId]: {
        ...Object.fromEntries(
          (panel?.local_filters ?? []).map((f) => [f.id, f.default]),
        ),
        ...(locals[panelId] ?? {}),
      },
    },
  };
  const previewRange = range ??
    draft?.spec.default_time_range ?? { kind: "relative", seconds: 3600 };
  let identity = "",
    referencedVariables: DashboardSchemas["DashboardVariable"][] = [];
  try {
    if (panel && draft) {
      referencedVariables = dashboardPreviewSpec(draft.spec, [
        panelId,
      ]).variables;
      identity = JSON.stringify([
        panelPreviewIdentity(panel, draft.spec, parameters),
        previewRange,
      ]);
    }
  } catch {
    /* Invalid dependencies are repairable in a personal draft. */
  }
  const identityRef = useRef(identity);
  identityRef.current = identity;
  const entry = execution?.panels.find((p) => p.id === panelId),
    target = panel?.targets.find((q) => q.id === queryId) ?? panel?.targets[0];
  const update = (patch: Partial<DashboardPanel>) => {
    const current = editor.current.current;
    if (current)
      editor.update({
        ...current,
        spec: {
          ...current.spec,
          panels: current.spec.panels.map((p) =>
            p.id === panelId ? { ...p, ...patch } : p,
          ),
        },
      });
  };
  const updateTarget = (
    patch: Partial<DashboardSchemas["DashboardTarget"]>,
  ) => {
    if (panel && target)
      update({
        targets: panel.targets.map((q) =>
          q.id === target.id ? { ...target, ...patch } : q,
        ),
      });
  };
  const run = async () => {
    if (!panel || !draft || editor.conflict || converting) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const attempt = ++sequence.current,
      captured = identityRef.current;
    setBusy(true);
    setError("");
    try {
      const saved = await editor.save();
      if (controller.signal.aborted) return;
      const result = await api.dashboards.sample(
        draftId,
        {
          expected_version: saved.draft_version,
          parameters: { ...parameters, ...bounds(previewRange) },
        },
        controller.signal,
      );
      if (
        controller.signal.aborted ||
        attempt !== sequence.current ||
        captured !== identityRef.current
      )
        return;
      setStatus(String(result.sample.status ?? "unavailable"));
      if (!result.validation.valid) {
        setError(
          result.validation.issues
            .map((i) => `${i.path}: ${i.message}`)
            .join("\n"),
        );
        return;
      }
      const executed = result.sample.execution as
        DashboardExecution | undefined;
      setExecution(executed);
      if (executed) {
        const effective = {
          ...parameters,
          variables: { ...parameters.variables, ...executed.variables },
          local_values: {
            ...parameters.local_values,
            ...executed.local_values,
          },
        };
        setVariables((previous) => ({ ...previous, ...executed.variables }));
        setLocals((previous) => ({ ...previous, ...executed.local_values }));
        setLastIdentity(
          JSON.stringify([
            panelPreviewIdentity(panel, draft.spec, effective),
            previewRange,
          ]),
        );
      } else setLastIdentity(captured);
    } catch (e) {
      if (!controller.signal.aborted && attempt === sequence.current)
        setError(
          formatApiError(e, t("dashboards.failed"), (id) =>
            t("common.requestReference", { requestId: id }),
          ),
        );
    } finally {
      if (attempt === sequence.current) setBusy(false);
    }
  };
  const runRef = useRef(run);
  runRef.current = run;
  useEffect(() => {
    request.current?.abort();
    sequence.current++;
    setBusy(false);
  }, [identity]);
  useEffect(
    () => () => {
      request.current?.abort();
      sequence.current++;
    },
    [],
  );
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault();
        void runRef.current();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  const dataKey = JSON.stringify([
    panel?.source_binding,
    target?.source_definition.builder?.metric,
    previewRange,
    parameters.resource_ids,
  ]);
  const { metadata, catalog } = useMetricMetadata(
    panel,
    target,
    { time: previewRange, resources: parameters.resource_ids ?? [] },
    updateTarget,
  );
  const resourceOptions = useMemo(
    () => [
      ...(hosts.data?.items ?? []).map((h) => ({ id: h.id, label: h.name })),
      ...(clusters.data?.items ?? []).map((c) => ({ id: c.id, label: c.name })),
    ],
    [hosts.data, clusters.data],
  );
  const convert = async (mode: "builder" | "dsl") => {
    if (!panel || converting || panel.authoring_mode === mode) return;
    setConverting(true);
    setError("");
    try {
      const result = await api.dashboards.convertPanel({ panel, mode });
      if (result.converted) update(result.panel);
      else
        setError(
          t("dashboards.conversionBlocked") +
            " " +
            result.issues.map((i) => i.message).join("; "),
        );
    } catch (e) {
      setError(
        formatApiError(e, t("dashboards.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setConverting(false);
    }
  };
  const generate = async () => {
    if (!panel) return;
    setConverting(true);
    setError("");
    try {
      let issues: DashboardSchemas["DashboardValidationReport"]["issues"] = [];
      await editor.mutate(async (saved) => {
        const result = await api.dashboards.generateDrilldowns(draftId, {
          expected_version: saved.draft_version,
          panel_id: panelId,
          signal_sources: {},
        });
        issues = result.issues;
        return result.draft;
      });
      setGenerated(true);
      if (issues.length) setError(issues.map((i) => i.message).join("; "));
    } catch (e) {
      setError(
        formatApiError(e, t("dashboards.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setConverting(false);
    }
  };
  const autoGenerated = useRef<string | undefined>(undefined);
  const generateRef = useRef(generate);
  generateRef.current = generate;
  const autoPanelId = panel?.id,
    autoDrillCount = panel?.drilldowns.length;
  useEffect(() => {
    if (
      autoPanelId &&
      !autoDrillCount &&
      autoGenerated.current !== autoPanelId
    ) {
      autoGenerated.current = autoPanelId;
      void generateRef.current();
    }
  }, [autoPanelId, autoDrillCount]);
  if (!canManage)
    return (
      <PageShell title={t("dashboards.editPanel")}>
        <Alert title={t("dashboards.noEdit")} description="" tone="warning" />
      </PageShell>
    );
  if (!draft || !panel || !target)
    return (
      <PageShell title={t("dashboards.editPanel")}>
        <p role="status">
          {draft ? t("dashboards.editor.panelMissing") : t("common.loading")}
        </p>
      </PageShell>
    );
  const builder = target.source_definition.builder,
    types =
      panel.signal === "metrics"
        ? metricCharts
        : panel.signal === "logs"
          ? ["logs", "timeseries", "stat", "bar", "pie", "table"]
          : traceCharts;
  const patchBuilder = (patch: Partial<DashboardSchemas["DashboardBuilder"]>) =>
    updateTarget({ source_definition: { builder: { ...builder!, ...patch } } });
  const chartChoices = observationChartCapabilities(
    panel.signal,
    types,
    lastIdentity === identity ? entry?.targets : undefined,
    panel.targets,
    panel.display,
  );
  const finish = async () => {
    if (!panel.title.trim()) {
      setError(t("dashboards.editor.invalidForm"));
      return;
    }
    try {
      await editor.save();
      await navigate({ to: "/dashboard-drafts/$draftId", params: { draftId } });
    } catch (e) {
      editor.setError(e);
    }
  };
  return (
    <PageShell
      className="argus-panel-editor"
      title={
        <span className="argus-panel-editor__title">
          <span className="argus-sr-only">{t("dashboards.editPanel")}</span>
          <Input
            appearance="title"
            aria-label={t("dashboards.panelTitle")}
            value={panel.title}
            maxLength={240}
            onChange={(e) => update({ title: e.target.value })}
          />
        </span>
      }
      leading={
        <Button
          variant="ghost"
          onPress={() =>
            void navigate({
              to: "/dashboard-drafts/$draftId",
              params: { draftId },
            })
          }
        >
          <ArrowLeft aria-hidden />
          {t("dashboards.editor.backToDashboard")}
        </Button>
      }
      actions={
        <>
          <span className="argus-panel-editor__save-status">
            <Badge>{t("dashboards.draft")}</Badge>
            {t(
              editor.saving
                ? "dashboards.saving"
                : editor.dirty
                  ? "dashboards.unsaved"
                  : "dashboards.saved",
            )}
          </span>
          <Button
            variant="primary"
            isDisabled={converting || editor.conflict}
            onPress={() => void finish()}
          >
            {t("dashboards.done")}
          </Button>
        </>
      }
    >
      {Boolean(error || editor.error) && (
        <Alert
          tone="danger"
          title={t("dashboards.failed")}
          description={
            error ||
            formatApiError(editor.error, t("dashboards.failed"), (requestId) =>
              t("common.requestReference", { requestId }),
            )
          }
        />
      )}
      <DashboardQueryScopeProvider
        value={{ time: previewRange, resources: parameters.resource_ids ?? [] }}
      >
        <div className="argus-panel-editor__workspace">
          <main className="argus-panel-editor__main">
            <section
              className="argus-panel-editor__preview"
              aria-label={t("dashboards.editor.previewTitle")}
            >
              <header>
                <div className="argus-action-group">
                  <strong>{t("dashboards.editor.previewTitle")}</strong>
                  <Badge>
                    {t(
                      lastIdentity === identity && lastIdentity
                        ? "dashboards.editor.lastResult"
                        : "dashboards.editor.pendingRun",
                    )}
                  </Badge>
                </div>
                <div className="argus-panel-editor__scope">
                  <TimeRangePicker
                    value={previewRange}
                    onChange={setRange}
                    label={t("dashboards.time")}
                  />
                  <ValueSelector
                    label={t("dashboards.resources")}
                    multiple
                    value={resources}
                    candidates={resourceOptions.map((r) => r.id)}
                    valueLabels={Object.fromEntries(
                      resourceOptions.map((r) => [r.id, r.label]),
                    )}
                    onChange={setResources}
                  />
                  <Button
                    variant="ghost"
                    onPress={() =>
                      setResultView(resultView === "chart" ? "table" : "chart")
                    }
                    aria-pressed={resultView === "table"}
                  >
                    <Table2 aria-hidden />
                    {t(
                      resultView === "chart"
                        ? "dashboards.editor.dataTable"
                        : "dashboards.editor.backToChart",
                    )}
                  </Button>
                </div>
              </header>
              <div className="argus-panel-editor__visual">
                <ObservationPanel
                  title={panel.title}
                  type={resultView === "table" ? "table" : panel.type}
                  signal={panel.signal}
                  targets={entry?.targets ?? []}
                  unit={panel.unit || metadata?.unit}
                  decimals={panel.decimals}
                  legend={panel.legend}
                  display={panel.display}
                  thresholds={panel.thresholds}
                  onSelect={(row, target) => {
                    if (execution?.context_token && lastIdentity === identity)
                      setDrill({ row, target, token: execution.context_token });
                  }}
                />
              </div>
              <footer>
                <span>{t("dashboards.editor.runHint")}</span>
                <span>{t("dashboards.editor.previewOnly")}</span>
                {status && (
                  <span role="status">
                    {t(`dashboards.statuses.${status}`, {
                      defaultValue: status,
                    })}{" "}
                    · {t("dashboards.sampleHint")}
                  </span>
                )}
              </footer>
            </section>
            <section
              className="argus-panel-editor__queries"
              aria-label={t("dashboards.queryTarget")}
            >
              <header>
                <strong>{t("dashboards.queryTarget")}</strong>
                <div className="argus-action-group">
                  <Select
                    ariaLabel={t("dashboards.queryTarget")}
                    value={target.id}
                    onValueChange={setQueryId}
                    options={panel.targets.map((q) => ({
                      value: q.id,
                      label: q.id,
                    }))}
                  />
                  <Button
                    isDisabled={panel.targets.length >= 8 || converting}
                    onPress={() => {
                      const q = {
                        ...newTarget(
                          panel.signal,
                          panel.type,
                          panel.authoring_mode as "builder" | "dsl",
                        ),
                        id: `query_${crypto.randomUUID().slice(0, 8)}`,
                      };
                      update({ targets: [...panel.targets, q] });
                      setQueryId(q.id);
                    }}
                  >
                    {t("dashboards.addQuery")}
                  </Button>
                  {panel.targets.length > 1 && (
                    <Button
                      variant="ghost"
                      onPress={() => {
                        update({
                          targets: panel.targets.filter(
                            (q) => q.id !== target.id,
                          ),
                        });
                        setQueryId("");
                      }}
                    >
                      {t("dashboards.delete")}
                    </Button>
                  )}
                  <SegmentedControl
                    label={t("dashboards.mode")}
                    disabled={converting}
                    value={panel.authoring_mode}
                    options={[
                      { value: "builder", label: t("dashboards.builder") },
                      {
                        value: "dsl",
                        label:
                          panel.signal === "metrics"
                            ? "PromQL"
                            : panel.signal === "logs"
                              ? "KQL"
                              : "Trace GraphQL",
                      },
                    ]}
                    onChange={(mode) => void convert(mode as "builder" | "dsl")}
                  />
                  {busy ? (
                    <Button
                      onPress={() => {
                        request.current?.abort();
                        sequence.current++;
                        setBusy(false);
                        setStatus("cancelled");
                      }}
                    >
                      {t("common.cancel")}
                    </Button>
                  ) : (
                    <Button
                      variant="primary"
                      isDisabled={converting || editor.conflict}
                      onPress={() => void run()}
                    >
                      <Play aria-hidden />
                      {t("dashboards.editor.run")}
                    </Button>
                  )}
                </div>
              </header>
              <fieldset
                disabled={converting}
                className="argus-panel-editor__query-fields"
              >
                <div className="argus-panel-editor__query-toolbar">
                  <Field label={t("dashboards.source")} requirement="required">
                    <Select
                      value={panel.source_binding.source_type}
                      options={sources[panel.signal].map((value) => ({
                        value,
                        label: t(`dashboards.editor.sources.${value}`, {
                          defaultValue: value,
                        }),
                      }))}
                      onValueChange={(source_type) =>
                        update({
                          source_binding: {
                            source_type,
                            capability_version: "v1",
                          },
                          targets: panel.targets.map((query) =>
                            query.source_definition.builder
                              ? {
                                  ...query,
                                  source_definition: {
                                    builder: {
                                      ...query.source_definition.builder,
                                      metric_type: undefined,
                                    },
                                  },
                                }
                              : query,
                          ),
                        })
                      }
                    />
                  </Field>
                  <p className="argus-dashboard-muted">
                    {t("dashboards.editor.inheritedScope")}
                  </p>
                </div>
                {builder && panel.signal === "metrics" && (
                  <div className="argus-panel-editor__basics">
                    <Field
                      label={t("dashboards.metric")}
                      requirement="required"
                    >
                      <ComboBox
                        value={builder.metric ?? ""}
                        allowCustom
                        options={(catalog.data?.metrics ?? []).map((m) => ({
                          value: m.name,
                          label: m.name,
                          description: [m.type, m.unit]
                            .filter(Boolean)
                            .join(" · "),
                        }))}
                        contextKey={dataKey}
                        fetchOptions={async (search, cursor, signal) => {
                          const result = await api.dashboards.catalog(
                            {
                              ...bounds(previewRange),
                              resource_ids: parameters.resource_ids ?? [],
                              signal: "metrics",
                              kind: "metrics",
                              source_binding: panel.source_binding,
                              selected_values: [],
                              filters: [],
                              search,
                              cursor,
                              limit: 100,
                            },
                            signal,
                          );
                          return {
                            options: result.metrics.map((m) => ({
                              value: m.name,
                              label: m.name,
                              description: [m.type, m.unit]
                                .filter(Boolean)
                                .join(" · "),
                            })),
                            next_cursor: result.next_cursor,
                          };
                        }}
                        onValueChange={(metric) =>
                          patchBuilder(selectMetric(builder, metric))
                        }
                      />
                    </Field>
                    <Field
                      label={t("dashboards.editor.calculation")}
                      requirement="required"
                    >
                      <Select
                        value={builder.operation}
                        options={[
                          "value",
                          "sum",
                          "avg",
                          "min",
                          "max",
                          "rate",
                          "error_rate",
                          "topk",
                          "p95",
                        ].map((value) => ({
                          value,
                          label: t(`dashboards.operations.${value}`),
                          disabled: !metricOperationCompatible(value, metadata),
                          description: requiredMetricType(value)
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
                            top_n:
                              operation === "topk"
                                ? (builder.top_n ?? 10)
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
                          })
                        }
                      />
                    </Field>
                    <Field
                      label={t("dashboards.editor.split")}
                      requirement="optional"
                    >
                      <ValueSelector
                        label={t("dashboards.editor.split")}
                        multiple
                        allLabel={t("dashboards.editor.noSplit")}
                        value={{
                          all: !builder.group_by?.length,
                          values: builder.group_by ?? [],
                        }}
                        candidates={metadata?.labels ?? []}
                        onChange={(selection) =>
                          patchBuilder({
                            group_by: selection.all ? [] : selection.values,
                          })
                        }
                      />
                    </Field>
                  </div>
                )}
                {metadata ? (
                  <p className="argus-dashboard-muted">
                    {metadata.type} ·{" "}
                    {metadata.unit || t("dashboards.editor.unknownUnit")}
                  </p>
                ) : (
                  builder &&
                  panel.signal === "metrics" && (
                    <p className="argus-dashboard-muted">
                      {t("dashboards.editor.metadataUnverified")}
                    </p>
                  )
                )}
                {builder &&
                  panel.signal === "metrics" &&
                  !metricOperationCompatible(builder.operation, metadata) && (
                    <p role="alert" className="argus-dashboard-muted">
                      {t("dashboards.editor.metricMismatch", {
                        type: metadata?.type,
                        required: requiredMetricType(builder.operation),
                      })}
                    </p>
                  )}
                {builder && panel.signal === "metrics" ? (
                  <>
                    <QueryFilterChips
                      key={target.id}
                      filters={builder.filters}
                      variables={draft.spec.variables}
                      locals={panel.local_filters.map((f) => f.id)}
                      onChange={(filters) => patchBuilder({ filters })}
                    />
                    <details className="argus-panel-editor__advanced">
                      <summary>{t("dashboards.editor.advancedQuery")}</summary>
                      <QueryTargetEditor
                        compactMetrics
                        key={target.id}
                        panel={panel}
                        spec={draft.spec}
                        target={target}
                        onChange={updateTarget}
                      />
                    </details>
                  </>
                ) : (
                  <QueryTargetEditor
                    key={target.id}
                    panel={panel}
                    spec={draft.spec}
                    target={target}
                    onChange={updateTarget}
                  />
                )}
                {draft.spec.variables.length > 0 && (
                  <details className="argus-panel-editor__advanced">
                    <summary>{t("dashboards.variables")}</summary>
                    {referencedVariables.map((v) => (
                      <ValueSelector
                        key={v.id}
                        label={v.label}
                        multiple={v.multiple}
                        value={parameters.variables?.[v.name] ?? v.default}
                        candidates={
                          execution?.variable_candidates[v.name]?.values ?? []
                        }
                        contextKey={candidateContext(v.query, {
                          time: previewRange,
                          resource_ids: parameters.resource_ids,
                          variables: parameters.variables,
                        })}
                        fetchValues={async (search, cursor, signal) => {
                          const result = await api.dashboards.catalog(
                            candidateRequest(
                              v.query,
                              {
                                ...bounds(previewRange),
                                resource_ids: parameters.resource_ids,
                                variables: parameters.variables,
                              },
                              parameters.variables?.[v.name] ?? v.default,
                              search,
                              cursor,
                            ),
                            signal,
                          );
                          return result;
                        }}
                        onChange={(selection) =>
                          setVariables((previous) => ({
                            ...previous,
                            [v.name]: selection,
                          }))
                        }
                      />
                    ))}
                  </details>
                )}
                {panel.local_filters.length > 0 && (
                  <details className="argus-panel-editor__advanced">
                    <summary>{t("dashboards.localFilters")}</summary>
                    {panel.local_filters.map((f) => (
                      <ValueSelector
                        key={f.id}
                        label={f.label}
                        multiple={f.multiple}
                        value={
                          parameters.local_values?.[panelId]?.[f.id] ??
                          f.default
                        }
                        candidates={
                          execution?.local_candidates[panelId]?.[f.id]
                            ?.values ?? []
                        }
                        contextKey={
                          f.query
                            ? candidateContext(f.query, {
                                time: previewRange,
                                resource_ids: parameters.resource_ids,
                                variables: parameters.variables,
                                locals: parameters.local_values?.[panelId],
                              })
                            : dataKey
                        }
                        fetchValues={
                          f.query
                            ? (search, cursor, signal) =>
                                api.dashboards.catalog(
                                  candidateRequest(
                                    f.query!,
                                    {
                                      ...bounds(previewRange),
                                      resource_ids: parameters.resource_ids,
                                      variables: parameters.variables,
                                      locals:
                                        parameters.local_values?.[panelId],
                                    },
                                    parameters.local_values?.[panelId]?.[
                                      f.id
                                    ] ?? f.default,
                                    search,
                                    cursor,
                                  ),
                                  signal,
                                )
                            : undefined
                        }
                        onChange={(selection) =>
                          setLocals((previous) => ({
                            ...previous,
                            [panelId]: {
                              ...previous[panelId],
                              [f.id]: selection,
                            },
                          }))
                        }
                      />
                    ))}
                  </details>
                )}
              </fieldset>
            </section>
          </main>
          <aside
            className="argus-panel-editor__settings"
            aria-label={t("dashboards.editor.settings")}
          >
            <Tabs value={tab} onValueChange={setTab}>
              <TabsList>
                <TabsTrigger value="chart">{t("dashboards.chart")}</TabsTrigger>
                <TabsTrigger value="style">
                  {t("dashboards.editor.style")}
                </TabsTrigger>
                <TabsTrigger value="interaction">
                  {t("dashboards.editor.interaction")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="chart">
                <strong>{t("dashboards.editor.suggestedCharts")}</strong>
                <p className="argus-dashboard-muted">
                  {t("dashboards.editor.chartSuggestionHint")}
                </p>
                <ChartTypePicker
                  value={panel.type}
                  options={chartChoices.map((choice) => ({
                    ...choice,
                    label: t(`dashboards.charts.${choice.type}`),
                    reason: t(
                      `dashboards.editor.chartReasons.${choice.reason}`,
                    ),
                  }))}
                  onChange={(type) =>
                    update({ type, ...compatibleDisplay(panel, type) })
                  }
                />
                <div className="argus-panel-editor__auto-unit">
                  <span>{t("dashboards.unit")}</span>
                  <strong>
                    {panel.unit ||
                      metadata?.unit ||
                      t("dashboards.display.auto")}
                  </strong>
                </div>
                <p className="argus-dashboard-muted">
                  {t("dashboards.editor.chartHint")}
                </p>
                {chartChoices.every(
                  (choice) => choice.state === "unverified",
                ) && (
                  <p className="argus-dashboard-muted">
                    {t("dashboards.editor.chartReasons.runFirst")}
                  </p>
                )}
              </TabsContent>
              <TabsContent value="style">
                <Field label={t("dashboards.unit")} requirement="optional">
                  <Select
                    value={panel.unit}
                    options={[
                      { value: "", label: t("dashboards.display.auto") },
                      ...new Set([
                        "percent",
                        "percent_ratio",
                        "bytes",
                        "s",
                        "ms",
                        "requests",
                        ...(panel.unit ? [panel.unit] : []),
                        ...(metadata?.unit ? [metadata.unit] : []),
                      ]),
                    ].map((v) =>
                      typeof v === "string"
                        ? {
                            value: v,
                            label: t(`dashboards.units.${v}`, {
                              defaultValue: v,
                            }),
                          }
                        : v,
                    )}
                    onValueChange={(unit) => update({ unit })}
                  />
                </Field>
                <Field label={t("dashboards.decimals")} requirement="required">
                  <Input
                    type="number"
                    min={0}
                    max={8}
                    value={panel.decimals}
                    onChange={(e) =>
                      update({ decimals: Number(e.target.value) })
                    }
                  />
                </Field>
                <Checkbox
                  isSelected={panel.legend}
                  onChange={(legend) => update({ legend })}
                >
                  {t("dashboards.legend")}
                </Checkbox>
                <DisplayEditor panel={panel} onChange={update} />
              </TabsContent>
              <TabsContent value="interaction">
                <p className="argus-dashboard-muted">
                  {t("dashboards.editor.drilldownHint")}
                </p>
                <Button isDisabled={converting} onPress={() => void generate()}>
                  {t(
                    generated
                      ? "dashboards.editor.regenerate"
                      : "dashboards.generate",
                  )}
                </Button>
                <DrilldownsEditor
                  panel={panel}
                  spec={draft.spec}
                  onChange={update}
                />
                <details className="argus-panel-editor__advanced">
                  <summary>{t("dashboards.localFilters")}</summary>
                  <LocalFiltersEditor
                    panel={panel}
                    spec={draft.spec}
                    onChange={(local_filters) => update({ local_filters })}
                  />
                </details>
                <details className="argus-panel-editor__advanced">
                  <summary>{t("dashboards.signal")}</summary>
                  <Select
                    value={panel.signal}
                    options={["metrics", "logs", "traces"].map((value) => ({
                      value,
                      label: t(`dashboards.editor.signals.${value}`),
                    }))}
                    onValueChange={(signal) => {
                      if (isDashboardSignal(signal) && signal !== panel.signal)
                        setReplace(signal);
                    }}
                  />
                </details>
              </TabsContent>
            </Tabs>
          </aside>
        </div>
      </DashboardQueryScopeProvider>
      <ConfirmDialog
        open={!!replace}
        onOpenChange={(open) => !open && setReplace(undefined)}
        title={t("dashboards.newQuery")}
        description={t("dashboards.reconfigureHint")}
        onConfirm={() => {
          if (!replace) return;
          const type =
            replace === "metrics"
              ? "timeseries"
              : replace === "logs"
                ? "logs"
                : "trace_list";
          update({
            signal: replace,
            type,
            source_binding: {
              source_type: sources[replace][0]!,
              capability_version: "v1",
            },
            targets: [
              newTarget(
                replace,
                type,
                panel.authoring_mode as "builder" | "dsl",
              ),
            ],
            detail_query_targets: [],
            drilldowns: [],
            local_filters: [],
            display: undefined,
            thresholds: [],
          });
          setReplace(undefined);
          autoGenerated.current = undefined;
        }}
      />
      {drill && (
        <DrilldownViewer
          dashboardId={draft.dashboard_id ?? ""}
          draft={{ id: draftId, version: draft.draft_version }}
          panel={panel}
          token={drill.token}
          initialRow={drill.row}
          initialTarget={drill.target}
          onClose={() => setDrill(undefined)}
        />
      )}
    </PageShell>
  );
}
