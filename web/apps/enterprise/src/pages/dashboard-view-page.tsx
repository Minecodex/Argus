import { useEffect, useState } from "react";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type DashboardPanel,
  type DashboardRevision,
  type DashboardSchemas,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  DashboardGrid,
  Dialog,
  Field,
  Input,
  ObservationPanel,
  PageShell,
  ValueSelector,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { PanelTile } from "../components/dashboards/panel-tile";
import { DrilldownViewer } from "../components/dashboards/drilldown-viewer";
import { DashboardBindingSummary } from "../components/dashboards/dashboard-binding-summary";
import "../styles/dashboards.css";
import { TimeRangePicker } from "../components/dashboards/time-range-picker";
import { candidateFilters } from "../components/dashboards/candidate-filters";
import {
  SourceSummary,
  sourceValueLabels,
} from "../components/dashboards/source-summary";
import {
  hasUnverifiedCandidates,
  panelCandidateResources,
} from "../components/dashboards/execution-status";

import { useDashboardExecution } from "../components/dashboards/use-dashboard-execution";

export function DashboardViewPage() {
  const { dashboardId } = useParams({
    from: "/authed/admin/dashboards/$dashboardId",
  });
  const search = useSearch({ from: "/authed/admin/dashboards/$dashboardId" });
  const api = useApi(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const canRead = usePermission("telemetry.dashboard.read"),
    canManage = usePermission("telemetry.dashboard.manage");
  const detail = useQuery({
    queryKey: ["dashboard", dashboardId],
    queryFn: () => api.dashboards.get(dashboardId),
    enabled: canRead,
  });
  const { refetch: refetchDetail } = detail;
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
  const [history, setHistory] = useState<DashboardRevision[] | null>(null),
    [historical, setHistorical] = useState<DashboardRevision | null>(null),
    [queryPanel, setQueryPanel] = useState<DashboardPanel>(),
    [expanded, setExpanded] = useState<DashboardPanel>(),
    [resourcesOpen, setResourcesOpen] = useState(false),
    [resourcesDraft, setResourcesDraft] = useState<string[]>([]);
  const [drill, setDrill] = useState<{
    panel: DashboardPanel;
    row: Record<string, unknown>;
    target: string;
    token: string;
  }>();
  const {
    execution,
    input,
    timeRange,
    error,
    setError,
    busy,
    bounds,
    latest,
    tokens,
    run,
    apply,
    applyVariable,
  } = useDashboardExecution({
    dashboardId,
    revision: detail.data?.revision,
    resourceId: search.resource,
    enabled: canRead && !historical,
    execute: (id, input, signal) => api.dashboards.execute(id, input, signal),
    onRevisionChanged: refetchDetail,
    onReset: () => setDrill(undefined),
    formatError: (e) =>
      formatApiError(e, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      ),
  });
  const observed = execution?.resources ?? [];
  const resourceOptions = Array.from(
    new Map([
      ...(hosts.data?.items ?? []).map(
        (h) => [h.id, { id: h.id, label: h.name }] as const,
      ),
      ...(clusters.data?.items ?? []).map(
        (c) => [c.id, { id: c.id, label: c.name }] as const,
      ),
      ...observed
        .filter(
          (r) =>
            !hosts.data?.items.some((h) => h.id === r.id) &&
            !clusters.data?.items.some((c) => c.id === r.id),
        )
        .map((r) => [r.id, { id: r.id, label: r.id }] as const),
    ]).values(),
  );
  const view = historical ?? detail.data?.revision;
  if (!canRead)
    return (
      <PageShell title={t("dashboards.title")}>
        <p role="alert">{t("dashboards.forbidden")}</p>
      </PageShell>
    );
  if (!view || !detail.data)
    return (
      <PageShell title={t("dashboards.title")}>
        {detail.error ? (
          <Alert
            tone="danger"
            title={t("dashboards.failed")}
            description={formatApiError(
              detail.error,
              t("dashboards.failed"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
          />
        ) : (
          t("common.loading")
        )}
      </PageShell>
    );
  const edit = async () => {
    try {
      const draft = await api.dashboards.createDraft({
        dashboard_id: dashboardId,
        name: view.name,
        description: view.description,
        spec: view.spec,
        proposed_bindings: [],
      });
      await navigate({
        to: "/dashboard-drafts/$draftId",
        params: { draftId: draft.id },
      });
    } catch (e) {
      setError(
        formatApiError(e, t("dashboards.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    }
  };
  return (
    <PageShell
      title={view.name}
      description={
        <>
          <Badge>
            {t(
              historical
                ? "dashboards.readOnlyHistory"
                : "dashboards.published",
            )}
          </Badge>{" "}
          · R{view.revision_number} · {view.description}
        </>
      }
      actions={
        <div className="argus-dashboard-inline">
          <Button onClick={() => void navigate({ to: "/dashboards" })}>
            {t("dashboards.back")}
          </Button>
          <Button
            onClick={() =>
              void api.dashboards
                .revisions(dashboardId)
                .then(setHistory)
                .catch((e) =>
                  setError(
                    formatApiError(e, t("dashboards.failed"), (requestId) =>
                      t("common.requestReference", { requestId }),
                    ),
                  ),
                )
            }
          >
            {t("dashboards.history")}
          </Button>
          {canManage && (
            <Button
              variant="primary"
              disabled={detail.data.dashboard.lifecycle !== "active"}
              onClick={() => void edit()}
            >
              {t("dashboards.edit")}
            </Button>
          )}
        </div>
      }
    >
      {import.meta.env.VITE_API_MODE === "mock" && (
        <Alert
          tone="info"
          title={t("dashboards.sample")}
          description={t("dashboards.mock")}
        />
      )}
      {error && (
        <Alert
          tone="danger"
          title={t("dashboards.failed")}
          description={error}
        />
      )}
      {historical ? (
        <Button onClick={() => setHistorical(null)}>
          {t("dashboards.currentVersion")}
        </Button>
      ) : (
        <div className="argus-dashboard-toolbar">
          <TimeRangePicker
            label={t("dashboards.time")}
            value={timeRange}
            onChange={(range) => {
              void run(latest.current, undefined, range);
            }}
          />
          <Button
            onClick={() => {
              setResourcesDraft(
                input.resource_ids?.length
                  ? input.resource_ids
                  : resourceOptions.map((r) => r.id),
              );
              setResourcesOpen(true);
            }}
          >
            {t("dashboards.resources")}:{" "}
            {input.resource_ids?.length || t("dashboards.all")}
          </Button>
          <Button disabled={busy} onClick={() => void run(latest.current)}>
            {t(busy ? "dashboards.refreshing" : "dashboards.refresh")}
          </Button>
        </div>
      )}
      {!historical && view.spec.variables.length > 0 && (
        <div className="argus-dashboard-filters">
          {view.spec.variables.map((variable) => {
            const selection =
              input.variables?.[variable.name] ?? variable.default;
            const candidates = execution?.variable_candidates[variable.name];
            return (
              <Field
                key={variable.id}
                label={variable.label}
                requirement="optional"
              >
                <ValueSelector
                  label={variable.label}
                  disabled={busy}
                  contextKey={JSON.stringify([
                    bounds.current,
                    input.resource_ids,
                    input.variables,
                  ])}
                  value={selection}
                  multiple={variable.multiple}
                  candidates={candidates?.values ?? []}
                  onChange={(value) => applyVariable(variable.name, value)}
                  fetchValues={async (search, cursor, signal) => {
                    const filters = candidateFilters(
                      variable.query,
                      latest.current.variables ?? {},
                    );
                    const result = await api.dashboards.catalog(
                      {
                        source_binding: variable.query.source_binding,
                        signal: variable.query.signal as
                          "metrics" | "logs" | "traces",
                        kind: "values",
                        field: variable.query.field,
                        metric: variable.query.metric,
                        resource_ids: latest.current.resource_ids ?? [],
                        selected_values: selection.values,
                        filters,
                        from: bounds.current!.from,
                        to: bounds.current!.to,
                        search,
                        cursor,
                        limit: 100,
                      },
                      signal,
                    );
                    return result;
                  }}
                />
              </Field>
            );
          })}
        </div>
      )}
      {!historical && hasUnverifiedCandidates(execution) && (
        <Alert
          tone="warning"
          title={t("dashboards.statuses.unavailable")}
          description={t("dashboards.candidateFailed")}
        />
      )}
      {!historical &&
        Object.values(execution?.variable_candidates ?? {}).some(
          (v) => v.reset,
        ) && (
          <p role="status" className="argus-dashboard-muted">
            {t("dashboards.candidateReset")}
          </p>
        )}
      <DashboardGrid
        items={view.spec.panels}
        rowHeight={view.spec.layout.row_height}
      >
        {(panel) => (
          <>
            <PanelTile
              panel={panel}
              execution={
                historical
                  ? undefined
                  : execution?.panels.find((p) => p.id === panel.id)
              }
              onExpand={() => setExpanded(panel)}
              onSelect={
                historical
                  ? undefined
                  : (row, target) => {
                      const token = tokens.current.get(panel.id);
                      if (token) setDrill({ panel, row, target, token });
                    }
              }
            />
            <div className="argus-dashboard-panel-footer">
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setQueryPanel(panel)}
              >
                {t("dashboards.readQuery")}
              </Button>
              {!historical && (
                <SourceSummary
                  title={panel.title}
                  sources={
                    execution?.panels.find((item) => item.id === panel.id)
                      ?.sources ?? []
                  }
                  resourceNames={Object.fromEntries(
                    resourceOptions.map((r) => [r.id, r.label]),
                  )}
                />
              )}
              {!historical &&
                panel.local_filters.map((filter) => {
                  const selection =
                    input.local_values?.[panel.id]?.[filter.id] ??
                    filter.default;
                  const update = (
                    value: import("@argus/api-client").DashboardSelection,
                  ) =>
                    apply(
                      {
                        ...latest.current,
                        local_values: {
                          ...latest.current.local_values,
                          [panel.id]: {
                            ...latest.current.local_values?.[panel.id],
                            [filter.id]: value,
                          },
                        },
                      },
                      [panel.id],
                    );
                  return filter.kind === "query" && filter.query ? (
                    <ValueSelector
                      key={filter.id}
                      label={`${panel.title} ${filter.label}`}
                      contextKey={JSON.stringify([
                        bounds.current,
                        input.resource_ids,
                        input.variables,
                        input.local_values?.[panel.id],
                      ])}
                      disabled={busy}
                      value={selection}
                      multiple={filter.multiple}
                      valueLabels={
                        filter.query.field === "source_id"
                          ? sourceValueLabels(
                              execution?.local_candidates[panel.id]?.[filter.id]
                                ?.sources ?? [],
                              t,
                            )
                          : undefined
                      }
                      candidates={
                        execution?.local_candidates[panel.id]?.[filter.id]
                          ?.values ?? []
                      }
                      onChange={update}
                      fetchValues={async (search, cursor, signal) => {
                        const query = filter.query!;
                        const resourceIds = panelCandidateResources(
                          execution,
                          panel,
                        );
                        if (!resourceIds.length) return { values: [] };
                        return api.dashboards.catalog(
                          {
                            signal: query.signal as
                              "metrics" | "logs" | "traces",
                            source_binding: query.source_binding,
                            kind: "values",
                            field: query.field,
                            metric: query.metric,
                            resource_ids: resourceIds,
                            from: bounds.current!.from,
                            to: bounds.current!.to,
                            filters: candidateFilters(
                              query,
                              latest.current.variables ?? {},
                              latest.current.local_values?.[panel.id] ?? {},
                            ),
                            selected_values: selection.values,
                            search,
                            cursor,
                            limit: 100,
                          },
                          signal,
                        );
                      }}
                    />
                  ) : (
                    <LocalValue
                      key={filter.id}
                      filter={filter}
                      title={panel.title}
                      selection={selection}
                      disabled={busy}
                      onChange={update}
                    />
                  );
                })}
            </div>
          </>
        )}
      </DashboardGrid>
      {!historical && <DashboardBindingSummary id={dashboardId} />}
      {!view.spec.panels.length && (
        <div className="argus-dashboard-empty">
          <h2>{t("dashboards.noPanels")}</h2>
          <p>{t("dashboards.noPanelsHint")}</p>
        </div>
      )}
      <Dialog
        open={resourcesOpen}
        onOpenChange={setResourcesOpen}
        title={t("dashboards.chooseResources")}
        footer={
          <>
            <Button
              onClick={() => {
                apply({ ...latest.current, resource_ids: [] });
                setResourcesOpen(false);
              }}
            >
              {t("dashboards.all")}
            </Button>
            <Button
              variant="primary"
              disabled={!resourcesDraft.length}
              onClick={() => {
                apply({ ...latest.current, resource_ids: resourcesDraft });
                setResourcesOpen(false);
              }}
            >
              {t("dashboards.apply")}
            </Button>
          </>
        }
      >
        <div className="argus-dashboard-form-stack">
          {resourceOptions.map((r) => (
            <label className="argus-dashboard-check" key={r.id}>
              <input
                type="checkbox"
                checked={resourcesDraft.includes(r.id)}
                onChange={(e) =>
                  setResourcesDraft(
                    e.target.checked
                      ? [...resourcesDraft, r.id]
                      : resourcesDraft.filter((id) => id !== r.id),
                  )
                }
              />
              {r.label}
            </label>
          ))}
        </div>
      </Dialog>
      <Dialog
        open={Boolean(history)}
        onOpenChange={(open) => !open && setHistory(null)}
        title={t("dashboards.history")}
      >
        <div className="argus-dashboard-form-stack">
          {history?.map((r) => (
            <div className="argus-dashboard-inline" key={r.id}>
              <span>
                R{r.revision_number} · {r.created_at}
              </span>
              <Button
                size="sm"
                onClick={() => {
                  setHistorical(r);
                  tokens.current.clear();
                  setDrill(undefined);
                  setExpanded(undefined);
                  setHistory(null);
                }}
              >
                {" "}
                {t("dashboards.showHistory")}
              </Button>
            </div>
          ))}
        </div>
      </Dialog>
      {queryPanel && (
        <Dialog
          open
          onOpenChange={(open) => !open && setQueryPanel(undefined)}
          title={`${queryPanel.title} · ${t("dashboards.readQuery")}`}
          size="lg"
        >
          <div className="argus-dashboard-query-review">
            {[...queryPanel.targets, ...queryPanel.detail_query_targets].map(
              (target) => (
                <section key={target.id}>
                  <strong>
                    {target.id} · {target.language}
                    {target.signal
                      ? ` · ${target.source_binding?.source_type}`
                      : ""}
                  </strong>
                  <pre>
                    {target.source_definition.dsl
                      ? [
                          target.source_definition.dsl.expression,
                          target.source_definition.dsl.pipeline,
                        ]
                          .filter(Boolean)
                          .join("\n| ")
                      : JSON.stringify(
                          target.source_definition.builder,
                          null,
                          2,
                        )}
                  </pre>
                  <details>
                    <summary>{t("dashboards.parameterBindings")}</summary>
                    <pre>
                      {JSON.stringify(
                        {
                          query_mode: target.query_mode,
                          range_step_policy: target.range_step_policy,
                          parameter_bindings: target.parameter_bindings,
                          operation: target.source_definition.dsl?.operation,
                          variables: target.source_definition.dsl?.variables,
                        },
                        null,
                        2,
                      )}
                    </pre>
                  </details>
                </section>
              ),
            )}
          </div>
        </Dialog>
      )}
      {expanded && (
        <Dialog
          open
          onOpenChange={(open) => !open && setExpanded(undefined)}
          title={expanded.title}
          width={1200}
        >
          <div className="argus-dashboard-detail-content">
            <ObservationPanel
              title={expanded.title}
              type={expanded.type}
              signal={expanded.signal}
              unit={expanded.unit}
              decimals={expanded.decimals}
              legend={expanded.legend}
              display={expanded.display}
              thresholds={expanded.thresholds}
              targets={
                (historical
                  ? undefined
                  : execution?.panels.find((p) => p.id === expanded.id)
                      ?.targets) ?? []
              }
            />
          </div>
        </Dialog>
      )}
      {drill && (
        <DrilldownViewer
          dashboardId={dashboardId}
          panel={drill.panel}
          token={drill.token}
          initialRow={drill.row}
          initialTarget={drill.target}
          onClose={() => setDrill(undefined)}
        />
      )}
    </PageShell>
  );
}

function LocalValue({
  filter,
  title,
  selection,
  disabled,
  onChange,
}: {
  filter: DashboardSchemas["DashboardLocalFilter"];
  title: string;
  selection: import("@argus/api-client").DashboardSelection;
  disabled: boolean;
  onChange: (value: import("@argus/api-client").DashboardSelection) => void;
}) {
  const [text, setText] = useState(selection.values.join(","));
  useEffect(() => setText(selection.values.join(",")), [selection]);
  return (
    <Input
      disabled={disabled}
      type={filter.kind === "number" ? "number" : "text"}
      aria-label={`${title} ${filter.label}`}
      placeholder={filter.label}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onKeyDown={(e) => {
        if (e.key !== "Enter") return;
        e.preventDefault();
        onChange(
          text
            ? {
                all: false,
                values: filter.multiple
                  ? text
                      .split(",")
                      .map((v) => v.trim())
                      .filter(Boolean)
                  : [text],
              }
            : { all: true, values: [] },
        );
      }}
    />
  );
}
