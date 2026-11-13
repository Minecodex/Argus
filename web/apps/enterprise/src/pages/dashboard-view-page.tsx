import {
  dashboardTimeFromSearch,
  dashboardTimeToSearch,
} from "../components/dashboards/url-time-range";
import { OperationalPanelContext } from "../components/dashboards/operational-panel-context";
import { dashboardKubernetesScope } from "../components/dashboards/url-kubernetes-scope";
import { EmbeddedDashboardFrame } from "../components/dashboards/embedded-dashboard-frame";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type DashboardPanel,
  type DashboardExecution,
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
  PageShell,
  ValueSelector,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { PanelTile } from "../components/dashboards/panel-tile";
import { ExpandedPanel } from "../components/dashboards/expanded-panel";
import { DrilldownViewer } from "../components/dashboards/drilldown-viewer";
import { DashboardLinkedResources } from "../components/dashboards/dashboard-linked-resources";
import { DashboardEntrySource } from "../components/dashboards/dashboard-entry-source";
import "../styles/dashboards.css";
import { DashboardControls } from "../components/dashboards/dashboard-controls";
import {
  candidateContext,
  candidateRequest,
} from "../components/dashboards/candidate-filters";
import {
  SourceSummary,
  sourceValueLabels,
} from "../components/dashboards/source-summary";
import {
  hasUnverifiedCandidates,
  panelCandidateResources,
} from "../components/dashboards/execution-status";

import {
  useDashboardExecution,
  type ViewInput,
} from "../components/dashboards/use-dashboard-execution";
import { ModelGovernanceControls } from "../components/dashboards/model-governance-controls";
import "../styles/settings.css";

export function DashboardViewPage() {
  const { dashboardId } = useParams({
    from: "/authed/admin/dashboards/$dashboardId",
  });
  const search = useSearch({ from: "/authed/admin/dashboards/$dashboardId" });
  const navigate = useNavigate();
  const { t } = useTranslation();
  const linkedScope = dashboardKubernetesScope(search);
  return (
    <DashboardView
      dashboardId={dashboardId}
      entry={search}
      fixedScope={linkedScope}
      scopeControls={
        linkedScope ? (
          <Field
            label={t("kubernetesResources.linkedScope")}
            requirement="optional"
            controlMode="group"
          >
            <span className="argus-dashboard-locked-scope">
              {t(
                linkedScope.kubernetes_scope.namespace_ids.length
                  ? "kubernetesResources.linkedNamespaces"
                  : "kubernetesResources.linkedCluster",
                { count: linkedScope.kubernetes_scope.namespace_ids.length },
              )}
            </span>
          </Field>
        ) : undefined
      }
      initialTime={dashboardTimeFromSearch(search)}
      onTimeChange={(time) =>
        void navigate({
          to: "/dashboards/$dashboardId",
          params: { dashboardId },
          search: { ...search, ...dashboardTimeToSearch(time) },
          replace: true,
          resetScroll: false,
        })
      }
    />
  );
}
export function DashboardView({
  dashboardId,
  entry = {},
  embedded = false,
  visible = true,
  fixedScope,
  initialTime,
  onTimeChange,
  scopeControls,
  executeOverride,
}: {
  dashboardId: string;
  entry?: {
    resource?: string;
    resource_type?: import("@argus/api-client").DashboardResourceType;
    model?: string;
    month?: string;
  };
  embedded?: boolean;
  visible?: boolean;
  fixedScope?: Partial<ViewInput>;
  initialTime?: DashboardSchemas["DashboardTimeRange"];
  onTimeChange?: (time: DashboardSchemas["DashboardTimeRange"]) => void;
  scopeControls?: ReactNode;
  executeOverride?: (
    id: string,
    input: DashboardSchemas["DashboardExecutionInput"],
    signal?: AbortSignal,
  ) => Promise<DashboardExecution>;
}) {
  const search = entry;
  const Frame = embedded ? EmbeddedDashboardFrame : PageShell;
  const api = useApi(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const canReadResources = usePermission("telemetry.dashboard.read"),
    canManage = usePermission("telemetry.dashboard.manage");
  const canReadModels = usePermission("model.usage.read");
  const canRead = canReadResources || canReadModels;
  const detail = useQuery({
    queryKey: ["dashboard", dashboardId],
    queryFn: () => api.dashboards.get(dashboardId),
    enabled: canRead,
  });
  const { refetch: refetchDetail } = detail;
  const isModelGovernance =
    detail.data?.dashboard.managed_template_key === "argus.model-governance";
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
    [expanded, setExpanded] = useState<{
      panel: DashboardPanel;
      revisionId: string;
      time: DashboardSchemas["DashboardTimeRange"];
      bounds?: { from: string; to: string };
      input: ViewInput;
      execution?: DashboardSchemas["DashboardPanelExecution"];
    }>();
  const [drill, setDrill] = useState<{
    panel: DashboardPanel;
    row: Record<string, unknown>;
    target: string;
    token: string;
  }>();
  const executeDashboard: typeof api.dashboards.execute = (id, input, signal) =>
    (executeOverride ?? api.dashboards.execute)(
      id,
      isModelGovernance
        ? {
            variables: input.variables,
            local_values: {},
            resource_ids: [],
            panel_ids: input.panel_ids,
            candidates_only: input.candidates_only,
          }
        : input,
      signal,
    );
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
    refreshSeconds,
    setRefreshSeconds,
  } = useDashboardExecution({
    dashboardId,
    revision: detail.data?.revision,
    resourceId: search.resource,
    resourceType: search.resource_type,
    modelId: isModelGovernance ? search.model : undefined,
    month: isModelGovernance ? search.month : undefined,
    enabled: canRead && !historical && visible,
    fixedScope,
    initialTime,
    onTimeChange,
    execute: executeDashboard,
    onRevisionChanged: refetchDetail,
    onReset: () => {
      setDrill(undefined);
    },
    formatError: (e) =>
      formatApiError(e, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      ),
  });
  const scopeIdentity = JSON.stringify(fixedScope ?? null);
  useEffect(() => setExpanded(undefined), [scopeIdentity, dashboardId]);
  const observed = execution?.resources ?? [];
  const view = historical ?? detail.data?.revision;
  const entryType =
    search.resource_type ??
    (hosts.data?.items.some((h) => h.id === search.resource)
      ? "host"
      : clusters.data?.items.some((c) => c.id === search.resource)
        ? "kubernetes_cluster"
        : observed.find((r) => r.id === search.resource)?.type);
  if (!canRead)
    return (
      <Frame title={t("dashboards.title")}>
        <p role="alert">{t("dashboards.forbidden")}</p>
      </Frame>
    );
  if (!view || !detail.data)
    return (
      <Frame title={t("dashboards.title")}>
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
      </Frame>
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
    <OperationalPanelContext.Provider
      value={{
        cluster: input.kubernetes_scope?.cluster_id,
        time: dashboardTimeToSearch(timeRange),
        namespaces: execution?.resources
          .filter((r) => r.type === "kubernetes_namespace")
          .map((r) => r.id),
        focus: (cluster, panel, uid) =>
          apply(
            {
              ...input,
              resource_mode: "explicit",
              kubernetes_scope: {
                cluster_id: cluster,
                namespace_ids:
                  execution?.resources
                    .filter((r) => r.type === "kubernetes_namespace")
                    .map((r) => r.id) ?? [],
                focus_uid: uid,
              },
            },
            [panel],
          ),
      }}
    >
      <Frame
        title={
          isModelGovernance
            ? t("modelGovernance.title")
            : detail.data.dashboard.managed_template_key ===
                "kubernetes.operations-overview"
              ? t("kubernetesResources.overviewTitle")
              : view.name
        }
        description={
          <>
            <Badge>
              {t(
                historical
                  ? "dashboards.readOnlyHistory"
                  : "dashboards.published",
              )}
            </Badge>{" "}
            · R{view.revision_number}
            {view.description && (
              <>
                {" "}
                ·{" "}
                {isModelGovernance
                  ? t("modelGovernance.managedHint")
                  : view.description}
              </>
            )}
            {search.resource && (
              <DashboardEntrySource
                key={search.resource}
                id={search.resource}
                type={entryType}
              />
            )}
          </>
        }
        actions={
          <div className="argus-dashboard-inline">
            <Button onPress={() => void navigate({ to: "/dashboards" })}>
              {t("dashboards.back")}
            </Button>
            <Button
              onPress={() =>
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
            {canManage && !isModelGovernance && (
              <Button
                variant="primary"
                isDisabled={detail.data.dashboard.lifecycle !== "active"}
                onPress={() => void edit()}
              >
                {t(
                  detail.data.dashboard.managed_template_key
                    ? "managedDashboards.copy"
                    : "dashboards.edit",
                )}
              </Button>
            )}
            {!historical && !isModelGovernance && (
              <DashboardLinkedResources id={dashboardId} />
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
        {!embedded && detail.data.dashboard.managed_template_key && (
          <Alert
            tone="info"
            title={t("managedDashboards.version", {
              version: detail.data.dashboard.managed_template_version,
            })}
            description={t(
              isModelGovernance
                ? "modelGovernance.managedHint"
                : "managedDashboards.hint",
            )}
          />
        )}
        {historical ? (
          <Button onPress={() => setHistorical(null)}>
            {t("dashboards.currentVersion")}
          </Button>
        ) : isModelGovernance ? (
          <ModelGovernanceControls
            execution={execution}
            variables={input.variables ?? {}}
            busy={busy}
            onVariable={(name, value) => {
              void run({
                ...latest.current,
                variables: { ...latest.current.variables, [name]: value },
              });
            }}
            onRefresh={() => void run(latest.current)}
          />
        ) : (
          <DashboardControls
            lockedScope={scopeControls}
            spec={view.spec}
            input={input}
            time={timeRange}
            bounds={bounds.current}
            execution={execution}
            busy={busy}
            refreshSeconds={refreshSeconds}
            onRefreshSeconds={setRefreshSeconds}
            onTime={(range) => void run(latest.current, undefined, range)}
            onScope={(scope) => apply({ ...latest.current, ...scope })}
            onVariable={applyVariable}
            onRefresh={() => void run(latest.current)}
          />
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
          items={
            detail.data.dashboard.managed_template_key
              ? view.spec.panels.map((panel) => ({
                  ...panel,
                  title: t(
                    isModelGovernance
                      ? `modelGovernance.panels.${panel.id}`
                      : detail.data.dashboard.managed_template_key ===
                          "kubernetes.operations-overview"
                        ? `kubernetesResources.overviewPanels.${panel.id}`
                        : `managedDashboards.metrics.${["host.processes", "host.systemd", "host.windows_services"].includes(detail.data.dashboard.managed_template_key!) ? detail.data.dashboard.managed_template_key!.replace(".", "_") : detail.data.dashboard.managed_template_key!.split(".")[0]}.${panel.id}`,
                    { defaultValue: panel.title },
                  ),
                }))
              : view.spec.panels
          }
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
                onExpand={() =>
                  setExpanded({
                    panel,
                    revisionId: view.id,
                    time: timeRange,
                    bounds:
                      historical || isModelGovernance || !execution
                        ? undefined
                        : { from: execution.from, to: execution.to },
                    input,
                    execution: historical
                      ? undefined
                      : execution?.panels.find((item) => item.id === panel.id),
                  })
                }
                onSelect={
                  historical
                    ? undefined
                    : (row, target) => {
                        const token = tokens.current.get(panel.id);
                        if (token) setDrill({ panel, row, target, token });
                      }
                }
              />
              {(!panel.data_source ||
                panel.data_source.kind === "telemetry") && (
                <div className="argus-dashboard-panel-footer">
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => setQueryPanel(panel)}
                  >
                    {t("dashboards.readQuery")}
                  </Button>
                  {!historical && !isModelGovernance && (
                    <SourceSummary
                      title={panel.title}
                      sources={
                        execution?.panels.find((item) => item.id === panel.id)
                          ?.sources ?? []
                      }
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
                          contextKey={candidateContext(filter.query, {
                            time: bounds.current,
                            resource_ids: input.resource_ids,
                            variables: input.variables,
                            locals: input.local_values?.[panel.id],
                          })}
                          disabled={busy}
                          value={selection}
                          multiple={filter.multiple}
                          valueLabels={
                            filter.query.field === "source_id"
                              ? sourceValueLabels(
                                  execution?.local_candidates[panel.id]?.[
                                    filter.id
                                  ]?.sources ?? [],
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
                              candidateRequest(
                                query,
                                {
                                  ...bounds.current!,
                                  resource_ids: resourceIds,
                                  variables: latest.current.variables,
                                  locals:
                                    latest.current.local_values?.[panel.id],
                                },
                                selection,
                                search,
                                cursor,
                              ),
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
              )}
            </>
          )}
        </DashboardGrid>
        {!view.spec.panels.length && (
          <div className="argus-dashboard-empty">
            <h2>{t("dashboards.noPanels")}</h2>
            <p>{t("dashboards.noPanelsHint")}</p>
          </div>
        )}
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
                  onPress={() => {
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
        {expanded && expanded.revisionId === view.id && (
          <ExpandedPanel
            panel={expanded.panel}
            revisionId={expanded.revisionId}
            initialTime={expanded.time}
            initialBounds={
              expanded.bounds ??
              (historical || isModelGovernance ? undefined : execution)
            }
            initialExecution={
              expanded.execution ??
              (historical
                ? undefined
                : execution?.panels.find(
                    (item) => item.id === expanded.panel.id,
                  ))
            }
            selectTime={!isModelGovernance}
            execute={
              historical
                ? undefined
                : (range, signal) =>
                    executeDashboard(
                      dashboardId,
                      {
                        ...expanded.input,
                        ...range,
                        panel_ids: [expanded.panel.id],
                      },
                      signal,
                    )
            }
            onClose={() => setExpanded(undefined)}
          />
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
      </Frame>
    </OperationalPanelContext.Provider>
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
