import "../../styles/operational-panels.css";
import { OperationalAlerts } from "./operational-alerts";
import { useContext } from "react";
import { OperationalPanelContext } from "./operational-panel-context";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { formatErrorCode } from "@argus/api-client";
import type {
  DashboardPanel,
  DashboardSchemas,
  KubernetesResourceSnapshot,
  KubernetesResourceReference,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Badge,
  Select,
  ResourceSummary,
  NodeAllocation,
  ResourceDistribution,
  ResourceGraph,
  ResourceTable,
  resourceKindName,
} from "@argus/ui";
const record = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
export function OperationalPanel({
  panel,
  execution,
  onResourceOpen,
  onFilter,
}: {
  panel: DashboardPanel;
  execution?: DashboardSchemas["DashboardPanelExecution"];
  onResourceOpen?: (cluster: string, ref: KubernetesResourceReference) => void;
  onFilter?: (cluster: string, kind: string, state?: string) => void;
}) {
  const { t, i18n } = useTranslation(),
    navigate = useNavigate(),
    context = useContext(OperationalPanelContext);
  const target = execution?.targets[0],
    payload = record(target?.data);
  const open = (cluster: string, ref: KubernetesResourceReference) => {
    if (onResourceOpen) {
      onResourceOpen(cluster, ref);
      return;
    }
    void navigate({
      to: "/kubernetes/$clusterId",
      params: { clusterId: cluster },
      search: {
        ...context.time,
        tab: "resources",
        kind: ref.type,
        namespace_id: ref.namespace_id,
        namespace_name: ref.namespace,
        resource_type: ref.type,
        resource_name: ref.name,
        resource_uid: ref.uid,
        resource_namespace: ref.namespace,
        resource_namespace_id: ref.namespace_id,
      },
    });
  };
  const filter = (cluster: string, kind: string, state?: string) => {
    if (onFilter) {
      onFilter(cluster, kind, state);
      return;
    }
    void navigate({
      to: "/kubernetes/$clusterId",
      params: { clusterId: cluster },
      search: {
        ...context.time,
        tab: "resources",
        kind,
        state_filter: state,
        namespace_id:
          context.namespaces?.length === 1 ? context.namespaces[0] : undefined,
        namespace_name:
          context.namespaces?.length === 1
            ? (
                payload.resources as
                  KubernetesResourceSnapshot["resources"] | undefined
              )?.find((row) => row.namespace_id === context.namespaces?.[0])
                ?.namespace
            : undefined,
      },
    });
  };
  if (!target)
    return (
      <p role="status">
        {t(
          execution?.status === "not_applicable"
            ? "kubernetesResources.notAvailable"
            : "common.loading",
        )}
      </p>
    );
  if (
    target.status === "error" ||
    target.status === "not_applicable" ||
    target.status === "skipped_budget"
  )
    return (
      <div>
        <Alert
          tone="warning"
          title={t("kubernetesResources.notAvailable")}
          description={
            target.code
              ? formatErrorCode(
                  target.code,
                  t("kubernetesResources.notAvailable"),
                )
              : t("kubernetesResources.noSource")
          }
        />
        {target.code === "KUBERNETES_OBJECT_IDENTITY_CHANGED" &&
          context.cluster &&
          context.focus && (
            <Button
              onPress={() => context.focus?.(context.cluster!, panel.id, "")}
            >
              {t("kubernetesResources.reselect")}
            </Button>
          )}
      </div>
    );
  if (panel.data_source?.kind === "alert_instances") {
    const items = Array.isArray(payload.items)
      ? (payload.items as {
          id: string;
          name: string;
          severity: string;
          lifecycle: string;
        }[])
      : [];
    return (
      <OperationalAlerts
        items={items}
        total={typeof payload.total === "number" ? payload.total : items.length}
        complete={payload.complete !== false}
        summary={panel.type === "alert_summary"}
        pageSize={panel.data_source.limit || 20}
      />
    );
  }
  const snapshot = target.data as KubernetesResourceSnapshot | undefined;
  if (!snapshot || !Array.isArray(snapshot.resources))
    return <p>{t("kubernetesResources.notAvailable")}</p>;
  const onSelect = (uid: string) => {
    const row = snapshot.resources.find((item) => item.uid === uid);
    if (row) open(snapshot.cluster_id, row);
  };
  return (
    <div
      className={
        "argus-operational-panel argus-operational-panel--" + panel.type
      }
    >
      {panel.type === "resource_summary" && (
        <ResourceSummary
          types={panel.data_source?.resource_types}
          snapshot={snapshot}
          onFilter={(kind, state) => filter(snapshot.cluster_id, kind, state)}
        />
      )}
      {panel.type === "node_allocation" && (
        <NodeAllocation snapshot={snapshot} onSelect={onSelect} />
      )}
      {panel.type === "resource_distribution" && (
        <ResourceDistribution
          snapshot={snapshot}
          onFilter={(kind, state) => filter(snapshot.cluster_id, kind, state)}
        />
      )}
      {panel.type === "resource_graph" && (
        <>
          {context.focus && (
            <Select
              ariaLabel={t("kubernetesResources.focusResource")}
              value={snapshot.focus_uid ?? ""}
              options={(snapshot.focus_candidates ?? []).map((row) => ({
                value: row.uid,
                label:
                  resourceKindName(row.type) +
                  " · " +
                  row.namespace +
                  " / " +
                  row.name,
              }))}
              onValueChange={(uid) =>
                context.focus?.(snapshot.cluster_id, panel.id, uid)
              }
            />
          )}
          {snapshot.errors.graph ? (
            <Alert
              tone="warning"
              title={t("kubernetesResources.graphBudget")}
              description={t("kubernetesResources.graphBudgetHint")}
            />
          ) : (
            <ResourceGraph snapshot={snapshot} onSelect={onSelect} />
          )}
        </>
      )}
      {["resource_table", "resource_events"].includes(panel.type) && (
        <ResourceTable
          snapshot={snapshot}
          events={panel.type === "resource_events"}
          pageSize={panel.data_source?.limit ?? 20}
          onSelect={onSelect}
        />
      )}
      <div className="argus-operational-panel__metadata">
        {!snapshot.complete && (
          <Badge tone="warning">{t("kubernetesResources.partial")}</Badge>
        )}
        <small className="argus-k8s-observation-time">
          {t("kubernetesResources.snapshot")} ·{" "}
          {new Date(snapshot.observed_at).toLocaleTimeString(i18n.language)}
        </small>
      </div>
    </div>
  );
}
