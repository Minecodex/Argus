import { useEffect, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type DashboardPanel,
  type DashboardExecution,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Field,
  Input,
  PageShell,
  Select,
  ValueSelector,
} from "@argus/ui";
import { useDashboardDraftWorkspace } from "./draft-workspace-context";
import { OperationalPanel } from "./operational-panel";
import { NamespaceSelect } from "../kubernetes/namespace-select";
import type { NamespaceChoice } from "../kubernetes/namespace-picker";
export const operationalViews = [
  "resource_summary",
  "node_allocation",
  "resource_distribution",
  "resource_graph",
  "resource_table",
  "resource_events",
  "alert_summary",
  "alert_list",
] as const;
export const resourceStates = [
  "unknown",
  "healthy",
  "configured",
  "progressing",
  "pending",
  "warning",
  "failed",
  "completed",
  "paused",
  "scaled_down",
  "terminating",
];
export function OperationalPanelEditor({ panelId }: { panelId: string }) {
  const api = useApi(),
    editor = useDashboardDraftWorkspace(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const draft = editor.draft,
    panel = draft?.spec.panels.find((p) => p.id === panelId);
  const [cluster, setCluster] = useState(""),
    [namespace, setNamespace] = useState<NamespaceChoice>(),
    [execution, setExecution] = useState<DashboardExecution>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const request = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => request.current?.abort(), []);
  const clusters = useQuery({
    queryKey: ["clusters", "dashboard-selector"],
    queryFn: () => api.kubernetes.listClusters(),
  });
  const scope = useQuery({
    queryKey: ["kubernetes-workbench", cluster, "scope"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.scope(cluster, signal),
    enabled: !!cluster,
  });
  const types = useQuery({
    queryKey: ["kubernetes-workbench", cluster, "types"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.types(cluster, signal),
    enabled: !!cluster,
  });
  const update = (patch: Partial<DashboardPanel>) => {
    const current = editor.current.current;
    if (current)
      editor.update({
        ...current,
        spec: {
          ...current.spec,
          schema_version: "argus.telemetry_dashboard/v2",
          panels: current.spec.panels.map((p) =>
            p.id === panelId ? { ...p, ...patch } : p,
          ),
        },
      });
    setExecution(undefined);
    request.current?.abort();
    setBusy(false);
  };
  const source = (
    patch: Partial<NonNullable<DashboardPanel["data_source"]>>,
  ) => {
    if (panel?.data_source)
      update({ data_source: { ...panel.data_source, ...patch } });
  };
  const run = async () => {
    if (!cluster || !panel || !draft) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    setExecution(undefined);
    try {
      const saved = await editor.save(),
        to = new Date(),
        ids = namespace
          ? [namespace.id]
          : scope.data?.cluster_wide
            ? [cluster]
            : (scope.data?.namespaces.map((n) => n.id) ?? []);
      if (!ids.length) throw new Error(t("kubernetesResources.notAvailable"));
      const result = await api.dashboards.sample(
        saved.id,
        {
          expected_version: saved.draft_version,
          parameters: {
            panel_ids: [panel.id],
            from: new Date(to.getTime() - 3600000).toISOString(),
            to: to.toISOString(),
            resource_mode: "explicit",
            resource_ids: ids,
            resource_types:
              namespace || !scope.data?.cluster_wide
                ? ["kubernetes_namespace"]
                : ["kubernetes_cluster"],
            kubernetes_scope: {
              cluster_id: cluster,
              namespace_ids: namespace
                ? [namespace.id]
                : scope.data?.cluster_wide
                  ? []
                  : ids,
            },
          },
        },
        controller.signal,
      );
      if (!controller.signal.aborted)
        setExecution(result.sample.execution as DashboardExecution | undefined);
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          formatApiError(cause, t("dashboards.failed"), (id) =>
            t("common.requestReference", { requestId: id }),
          ),
        );
    } finally {
      if (request.current === controller) setBusy(false);
    }
  };
  if (!panel || !draft) return null;
  return (
    <PageShell
      title={panel.title}
      actions={
        <Button
          onPress={() =>
            void navigate({
              to: "/dashboard-drafts/$draftId",
              params: { draftId: draft.id },
            })
          }
        >
          {t("common.back")}
        </Button>
      }
    >
      <div className="argus-dashboard-form-stack">
        <Alert
          tone="info"
          title={t("kubernetesResources.nativeEditor")}
          description={t("kubernetesResources.nativeEditorHint")}
        />
        <Field requirement="required" label={t("dashboards.panelTitle")}>
          <Input
            value={panel.title}
            onChange={(event) => update({ title: event.target.value })}
          />
        </Field>
        <Field
          requirement="required"
          label={t("kubernetesResources.visualization")}
        >
          <Select
            value={panel.type}
            onValueChange={(type) => {
              const kind = type.startsWith("alert_")
                ? "alert_instances"
                : "kubernetes_inventory";
              update({
                type,
                signal: kind === "alert_instances" ? "alerts" : "resources",
                source_binding: { source_type: kind, capability_version: "v1" },
                data_source: { kind, limit: 20 },
              });
            }}
            options={operationalViews.map((value) => ({
              value,
              label: t("kubernetesResources.views." + value),
            }))}
          />
        </Field>
        {!panel.type.startsWith("alert_") && (
          <>
            <ValueSelector
              label={t("kubernetesWorkbench.type")}
              allLabel={t("common.all")}
              value={{
                all: !panel.data_source?.resource_types?.length,
                values: panel.data_source?.resource_types ?? [],
              }}
              multiple
              candidates={(types.data?.items ?? [])
                .filter((row) => !row.key.startsWith("custom/"))
                .map((row) => row.key)}
              valueLabels={Object.fromEntries(
                (types.data?.items ?? []).map((row) => [row.key, row.kind]),
              )}
              onChange={(value) =>
                source({ resource_types: value.all ? [] : value.values })
              }
            />
            <ValueSelector
              label={t("kubernetesResources.status")}
              allLabel={t("common.all")}
              value={{
                all: !panel.data_source?.states?.length,
                values: panel.data_source?.states ?? [],
              }}
              multiple
              candidates={resourceStates}
              valueLabels={Object.fromEntries(
                resourceStates.map((value) => [
                  value,
                  t("kubernetesResources.states." + value),
                ]),
              )}
              onChange={(value) =>
                source({ states: value.all ? [] : value.values })
              }
            />
          </>
        )}
        <Field requirement="required" label={t("kubernetesResources.rowLimit")}>
          <Input
            type="number"
            min={1}
            max={1000}
            value={panel.data_source?.limit ?? 20}
            onChange={(event) => {
              const limit = Number(event.target.value);
              if (Number.isInteger(limit) && limit >= 1 && limit <= 1000)
                source({ limit });
            }}
          />
        </Field>
        <div className="argus-dashboard-inline">
          <Field
            requirement="required"
            label={t("kubernetesResources.previewCluster")}
          >
            <Select
              value={cluster}
              onValueChange={(value) => {
                request.current?.abort();
                setBusy(false);
                setCluster(value);
                setNamespace(undefined);
                setExecution(undefined);
              }}
              options={[
                { value: "", label: t("common.select") },
                ...(clusters.data?.items ?? []).map((row) => ({
                  value: row.id,
                  label: row.name,
                })),
              ]}
            />
          </Field>
          {scope.data && (
            <NamespaceSelect
              clusterId={cluster}
              scopeVersion={scope.data.cluster_version}
              value={namespace}
              onChange={(value) => {
                request.current?.abort();
                setBusy(false);
                setNamespace(value);
                setExecution(undefined);
              }}
            />
          )}
          <Button
            variant="primary"
            isDisabled={!scope.data || editor.conflict}
            isPending={busy}
            onPress={() => void run()}
          >
            {t("dashboards.editor.run")}
          </Button>
        </div>
        {error && (
          <Alert
            tone="danger"
            title={t("dashboards.failed")}
            description={error}
          />
        )}
        {execution && (
          <OperationalPanel
            panel={panel}
            execution={execution.panels.find((row) => row.id === panelId)}
          />
        )}
      </div>
    </PageShell>
  );
}
