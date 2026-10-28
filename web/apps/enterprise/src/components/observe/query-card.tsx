import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowUp, ArrowDown, Copy, Play, X } from "lucide-react";
import {
  useApi,
  emptyDashboardSpec,
  type ObservationSignal,
  type ObservationCatalogInput,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardHeader,
  CardContent,
  Field,
  ObservationChartLink,
  ObservationPanel,
  ObservationScopePicker,
  ObservationWarnings,
  SegmentedControl,
  Select,
  Spinner,
  TelemetryEditorProvider,
  type QueryResourceType,
  type TelemetryCompletionRequest,
} from "@argus/ui";
import { QueryTargetEditor } from "../dashboards/query-target-editor";
import { useDefinitionForm } from "../dashboards/use-definition-form";
import { DashboardQueryScopeProvider } from "../dashboards/query-scope";
import {
  hasQueryResources,
  makeQuery,
  changeQueryScope,
  sameResourceSelection,
  type ObserveQuery,
} from "./model";
import type { ObserveWorkspace } from "./use-workspace";
import type { ObservationTimeRange } from "./time-range";

export function ObserveQueryCard({
  query: q,
  index,
  total,
  workspace: w,
  timeRange,
  time,
  actorKey,
  allowedKinds,
  linkGroup,
}: {
  query: ObserveQuery;
  index: number;
  total: number;
  workspace: ObserveWorkspace;
  timeRange: ObservationTimeRange;
  time: { from: string; to: string };
  actorKey: string;
  allowedKinds: QueryResourceType[];
  linkGroup?: string;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation();
  const form = useRef<HTMLFormElement>(null);
  const definitionForm = useDefinitionForm(
    q,
    () => form.current?.checkValidity() ?? true,
    t("observe.invalidFields"),
  );
  const registerValidation = w.registerValidation;
  const update = useRef(w.update);
  update.current = w.update;
  const [collapsed, setCollapsed] = useState(false);
  const contentId = useId();
  useEffect(
    () =>
      registerValidation(q.id, () => {
        const invalid = form.current?.querySelector<HTMLElement>(":invalid");
        const details = invalid?.closest("details");
        if (details) details.open = true;
        if (invalid && collapsed) {
          setCollapsed(false);
          requestAnimationFrame(() => form.current?.reportValidity());
          return false;
        }
        return form.current?.reportValidity() ?? true;
      }),
    [q.id, registerValidation, collapsed],
  );
  const selected = hasQueryResources(q.scope);
  const resourceKey = JSON.stringify({
    mode: q.scope.mode,
    resources: q.scope.resources,
  });
  const scopeKey = JSON.stringify([actorKey, q.scope, q.panel.signal, time]);
  const sources = useQuery({
    queryKey: ["observe-sources", actorKey, resourceKey, q.panel.signal, time],
    enabled: selected,
    gcTime: 0,
    queryFn: ({ signal }) =>
      api.telemetry.catalog(
        {
          scope: { mode: q.scope.mode, resources: q.scope.resources },
          signal: q.panel.signal,
          kind: "sources",
          ...time,
          limit: 100,
        },
        signal,
      ),
  });
  useEffect(() => {
    if (!q.scope.source_type && sources.data?.sources.length === 1) {
      const source = sources.data.sources[0]!;
      update.current(q.id, (old) => ({
        ...old,
        scope: { ...old.scope, source_type: source.source_type },
        panel: {
          ...old.panel,
          source_binding: {
            source_type: source.source_type,
            capability_version: source.capability_version,
          },
        },
      }));
    }
  }, [q.id, q.scope.source_type, sources.data]);
  const catalog = useMemo(() => {
    const load = (
      input: Omit<ObservationCatalogInput, "scope" | "signal">,
      signal?: AbortSignal,
    ) =>
      api.telemetry.catalog(
        { ...input, scope: q.scope, signal: q.panel.signal },
        signal,
      );
    const loadDashboard: import("../dashboards/query-scope").QueryCatalogLoader =
      async (input, signal) => {
        const {
          kind,
          metric,
          field,
          search,
          cursor,
          filters,
          selected_values,
        } = input;
        const result = await load(
          {
            kind,
            metric,
            field,
            search,
            cursor,
            filters,
            selected_values,
            limit: Math.min(100, input.limit),
            ...time,
          },
          signal,
        );
        return {
          metrics: result.metrics,
          fields: result.fields,
          values: result.values,
          has_more: result.has_more,
          next_cursor: result.next_cursor,
          complete: result.complete,
          selected_exists: result.selected_exists,
        };
      };
    return {
      loadDashboard,
      loadCompletion: (
        input: TelemetryCompletionRequest,
        signal: AbortSignal,
      ) => load({ ...time, ...input, limit: 100 }, signal),
    };
  }, [api, q.scope, q.panel.signal, time]);
  const ready =
    selected &&
    !!q.scope.source_type &&
    sources.data?.sources.some((s) => s.source_type === q.scope.source_type);
  const target = q.panel.targets[0]!,
    title = q.panel.title || t("observe.queryName", { number: index + 1 });
  const spec = {
    ...emptyDashboardSpec(),
    default_time_range: { kind: "absolute" as const, ...time },
    panels: [q.panel],
  };
  const busy = q.status === "queued" || q.status === "running";
  const resultType =
    q.view === "table"
      ? "table"
      : q.panel.signal === "metrics"
        ? target.query_mode === "instant"
          ? "table"
          : "timeseries"
        : q.panel.signal === "logs"
          ? q.result?.result_type === "log_entries"
            ? "logs"
            : "table"
          : q.panel.type;
  const detail = (row: Record<string, unknown>) => {
    const trace = row.traceId ?? row.trace_id;
    if (typeof trace !== "string") return;
    const next = makeQuery("traces", q.scope);
    next.panel.authoring_mode = "dsl";
    next.panel.title = `Trace ${trace.slice(0, 12)}`;
    const quote = (v: unknown) =>
      JSON.stringify(typeof v === "string" ? v : "");
    const source = row.sourceId ?? row.source_id,
      resource = row.resourceId ?? row.resource_id;
    const args = [
      `traceId:${quote(trace)}`,
      typeof source === "string" ? `sourceId:${quote(source)}` : "",
      typeof resource === "string" ? `resourceId:${quote(resource)}` : "",
    ]
      .filter(Boolean)
      .join(",");
    next.panel.type = q.panel.signal === "logs" ? "trace_list" : "trace_detail";
    next.panel.targets[0]!.source_definition = {
      dsl: {
        expression:
          q.panel.signal === "logs"
            ? `{queryTraces(${args},pageNum:1,pageSize:20){total traces{traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount}}}`
            : `{queryTrace(${args}){traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount completeness spans{spanId parentSpanId serviceName operationName startTime duration status attributes events links}}}`,
      },
    };
    w.add(next);
  };
  return (
    <Card className="argus-observe-query" data-query-id={q.id}>
      <CardHeader
        onToggle={() => setCollapsed((value) => !value)}
        expanded={!collapsed}
        controlsId={contentId}
        toggleLabel={t(collapsed ? "observe.expand" : "observe.collapse")}
        title={
          <div className="argus-observe-query-heading">
            <Badge tone="accent">{index + 1}</Badge>
            <strong>{title}</strong>
            <Select
              ariaLabel={t("observe.signal")}
              value={q.panel.signal}
              options={(["metrics", "logs", "traces"] as const).map(
                (value) => ({ value, label: t(`telemetry.${value}`) }),
              )}
              onValueChange={(v) => w.signal(q.id, v as ObservationSignal)}
            />
            <ObservationScopePicker
              value={q.scope}
              contextKey={actorKey}
              allowedKinds={allowedKinds}
              load={api.telemetry.resources}
              description={t("observe.scopeFor", { name: title })}
              onChange={(scope) => {
                if (!sameResourceSelection(q.scope, scope))
                  w.update(q.id, (old) => changeQueryScope(old, scope));
              }}
            />
            {!selected && (
              <span className="argus-observe-hint argus-observe-scope-hint">
                {t("observe.selectResources")}
              </span>
            )}
            {collapsed && <Badge>{t(`observe.status.${q.status}`)}</Badge>}
          </div>
        }
        action={
          <div className="argus-observe-actions">
            <Button
              variant="ghost"
              isIconOnly
              aria-label={t("observe.up")}
              isDisabled={index === 0}
              onPress={() => w.move(q.id, -1)}
            >
              <ArrowUp />
            </Button>
            <Button
              variant="ghost"
              isIconOnly
              aria-label={t("observe.down")}
              isDisabled={index === total - 1}
              onPress={() => w.move(q.id, 1)}
            >
              <ArrowDown />
            </Button>
            <Button
              variant="ghost"
              isIconOnly
              aria-label={t("observe.copy")}
              onPress={() => w.duplicate(q.id)}
            >
              <Copy />
            </Button>
            <Button
              variant="ghost"
              isIconOnly
              aria-label={t("observe.remove")}
              onPress={() => w.remove(q.id)}
            >
              <X />
            </Button>
          </div>
        }
      />
      <CardContent id={contentId} hidden={collapsed}>
        <div className="argus-observe-toolbar">
          <SegmentedControl
            disabled={busy || sources.isFetching}
            label={t("telemetry.queryConfiguration")}
            value={q.panel.authoring_mode}
            onChange={(mode) => w.switchMode(q.id, mode as "builder" | "dsl")}
            options={[
              { value: "builder", label: t("telemetry.builder") },
              { value: "dsl", label: t("telemetry.dslEditor") },
            ]}
          />
          <Button
            variant={busy ? "secondary" : "primary"}
            isDisabled={!busy && !ready}
            onPress={() => (busy ? w.cancel(q.id) : w.run([q.id], timeRange))}
          >
            {busy ? <X /> : <Play />}
            {t(busy ? "observe.cancel" : "telemetry.runQuery")}
          </Button>
        </div>
        {sources.isError && (
          <>
            <Alert
              tone="danger"
              title={t("observe.sourcesFailed")}
              description={t("observe.retrySources")}
            />
            <Button onPress={() => void sources.refetch()}>
              {t("observe.retry")}
            </Button>
          </>
        )}
        {selected && sources.data && !sources.data.sources.length && (
          <Alert
            title={t("observe.noSources")}
            description={t("observe.noSourcesHint")}
            tone="warning"
          />
        )}
        {q.conversionError && (
          <Alert
            title={t("observe.conversionFailed")}
            description={q.conversionError}
            tone="warning"
          />
        )}
        <DashboardQueryScopeProvider
          value={{
            key: scopeKey,
            time: spec.default_time_range,
            resources: q.scope.resources.map((r) => r.id),
            catalog: Boolean(ready),
            loadCatalog: catalog.loadDashboard,
          }}
        >
          <TelemetryEditorProvider
            value={{
              key: scopeKey,
              load: ready ? catalog.loadCompletion : undefined,
              onRun: () => w.run([q.id], timeRange),
            }}
          >
            <form
              ref={form}
              className="argus-observe-editor"
              onSubmit={definitionForm.handleSubmit(() =>
                w.run([q.id], timeRange),
              )}
            >
              <QueryTargetEditor
                transient
                metricLayout
                sourceControl={
                  <Field
                    className="argus-query-source"
                    label={t("observe.source")}
                    requirement="required"
                  >
                    <Select
                      ariaLabel={t("observe.source")}
                      value={q.scope.source_type ?? ""}
                      options={[
                        { value: "", label: t("observe.chooseSource") },
                        ...(sources.data?.sources ?? []).map((s) => ({
                          value: s.source_type,
                          label: s.source_type,
                        })),
                      ]}
                      disabled={!selected || sources.isFetching}
                      onValueChange={(value) =>
                        w.update(q.id, (old) => ({
                          ...old,
                          scope: {
                            ...old.scope,
                            source_type: value || undefined,
                            source_keys: undefined,
                          },
                          panel: {
                            ...old.panel,
                            source_binding: {
                              source_type: value,
                              capability_version:
                                sources.data?.sources.find(
                                  (s) => s.source_type === value,
                                )?.capability_version ?? "v1",
                            },
                          },
                        }))
                      }
                    />
                  </Field>
                }
                onInvalidInput={() => w.update(q.id, (old) => old)}
                onMetadataChange={(target) =>
                  w.annotate(q.id, (old) => ({
                    ...old,
                    panel: { ...old.panel, targets: [target] },
                  }))
                }
                panel={q.panel}
                target={target}
                spec={spec}
                onChange={(target) =>
                  w.update(q.id, (old) => ({
                    ...old,
                    panel: {
                      ...old.panel,
                      title:
                        target.source_definition.builder?.metric ||
                        old.panel.title,
                      targets: [target],
                    },
                  }))
                }
              />
            </form>
          </TelemetryEditorProvider>
        </DashboardQueryScopeProvider>
        {q.range && (
          <small className="argus-observe-hint">
            {new Date(q.range.from).toLocaleString(i18n.language)} –{" "}
            {new Date(q.range.to).toLocaleString(i18n.language)}
          </small>
        )}
        {q.compiled && (
          <details className="argus-observe-compiled">
            <summary>{t("telemetry.executedDsl")}</summary>
            <pre>{q.compiled}</pre>
          </details>
        )}
        <div className="argus-observe-toolbar">
          <div>
            {q.panel.signal === "metrics" && q.result && (
              <SegmentedControl
                label={t("telemetry.resultView")}
                value={q.view}
                onChange={(view) => w.setView(q.id, view as "graph" | "table")}
                options={[
                  { value: "graph", label: t("observe.graph") },
                  { value: "table", label: t("observe.table") },
                ]}
              />
            )}
          </div>
          <small role="status">
            {t(`observe.status.${q.status}`)}
            {q.result?.elapsed_ms !== undefined
              ? ` · ${q.result.elapsed_ms} ms`
              : ""}
          </small>
        </div>
        {busy && <Spinner label={t("observe.status.running")} />}
        {q.error && (
          <Alert
            title={t(
              q.status === "empty" ? "observe.noData" : "observe.failed",
            )}
            description={q.error}
            tone={q.status === "empty" ? "warning" : "danger"}
          />
        )}
        {q.result?.partial && (
          <Alert
            title={t("telemetry.partial")}
            description={t("observe.partialHint")}
            tone="warning"
          />
        )}
        <ObservationWarnings warnings={q.result?.warnings} />
        {q.result && (
          <ObservationChartLink
            group={q.panel.signal === "metrics" ? linkGroup : undefined}
          >
            <ObservationPanel
              title={title}
              signal={q.panel.signal}
              type={resultType}
              unit={q.panel.unit}
              range={q.range}
              targets={[
                {
                  id: "main",
                  status: q.result.partial ? "partial" : "success",
                  result_type: q.result.result_type,
                  data: q.result.data,
                },
              ]}
              onSelect={q.panel.signal === "metrics" ? undefined : detail}
            />
          </ObservationChartLink>
        )}
      </CardContent>
    </Card>
  );
}
