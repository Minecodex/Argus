import {
  dashboardTimeFromSearch,
  dashboardTimeToSearch,
} from "../dashboards/url-time-range";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  ApiError,
  type KubernetesInventoryScope,
  type DashboardSchemas,
} from "@argus/api-client";
import { Alert, Button, Spinner, Field } from "@argus/ui";
import { DashboardView } from "../../pages/dashboard-view-page";
import { usePermission } from "../../lib/permissions";
import { NamespaceSelect } from "./namespace-select";
import { OverviewDashboardSelector } from "./overview-dashboard-selector";
import { useResourceLocation } from "./resource-location";
import { dashboardKubernetesSearch } from "../dashboards/url-kubernetes-scope";
export function KubernetesClusterOverview({
  clusterId,
  access,
  visible,
}: {
  clusterId: string;
  access: KubernetesInventoryScope;
  visible: boolean;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    location = useResourceLocation(clusterId),
    [selecting, setSelecting] = useState(false),
    canManage = usePermission("kubernetes.manage");
  const selection = useQuery({
    queryKey: ["kubernetes-overview", clusterId],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.overview(clusterId, signal),
    enabled: visible,
    refetchOnWindowFocus: false,
  });
  const selected = location.search.namespace_id
    ? {
        id: location.search.namespace_id,
        name: location.search.namespace_name ?? "",
      }
    : undefined;
  const namespaceIds = selected
    ? [selected.id]
    : access.cluster_wide
      ? []
      : access.namespaces.map((row) => row.id);
  const fixedScope = {
    resource_mode: "explicit" as const,
    resource_ids: namespaceIds.length ? namespaceIds : [clusterId],
    resource_types: namespaceIds.length
      ? ["kubernetes_namespace" as const]
      : ["kubernetes_cluster" as const],
    kubernetes_scope: { cluster_id: clusterId, namespace_ids: namespaceIds },
  };
  const controls = (
    <Field
      label={t("kubernetesWorkbench.namespace")}
      requirement="optional"
      controlMode="group"
    >
      <NamespaceSelect
        clusterId={clusterId}
        scopeVersion={access.cluster_version}
        value={selected}
        onChange={(value) =>
          location.update({
            namespace_id: value?.id,
            namespace_name: value?.name,
          })
        }
      />
    </Field>
  );
  const board = selection.error ? undefined : selection.data?.dashboard;
  const expandQuery = new URLSearchParams({
    ...dashboardKubernetesSearch(fixedScope.kubernetes_scope),
  });
  for (const [key, value] of Object.entries(location.search))
    if (key.startsWith("time_") && value !== undefined)
      expandQuery.set(key, String(value));
  const execute = (
    id: string,
    input: DashboardSchemas["DashboardExecutionInput"],
    signal?: AbortSignal,
  ) =>
    api.kubernetesWorkbench
      .executeOverview(
        clusterId,
        {
          ...input,
          kubernetes_scope: {
            ...fixedScope.kubernetes_scope,
            focus_uid: input.kubernetes_scope?.focus_uid,
          },
          expected_dashboard_id: id,
        },
        signal,
      )
      .catch(async (cause) => {
        if (
          cause instanceof ApiError &&
          cause.code === "DASHBOARD_SELECTION_STALE"
        )
          await selection.refetch();
        throw cause;
      });
  return (
    <section className="argus-k8s-overview">
      <div className="argus-k8s-overview__header">
        <div>
          <strong>
            {board?.managed_template_key === "kubernetes.operations-overview"
              ? t("kubernetesResources.overviewTitle")
              : (board?.name ?? t("kubernetesResources.overviewTab"))}
          </strong>
          <p>{t("kubernetesResources.liveSnapshot")}</p>
        </div>
        <div className="argus-k8s-detail-actions">
          {board && (
            <Button
              variant="ghost"
              onPress={() =>
                window.open(
                  "/dashboards/" + board.id + "?" + expandQuery.toString(),
                  "_blank",
                  "noopener",
                )
              }
            >
              {t("kubernetesResources.expand")}
            </Button>
          )}
          {canManage && access.cluster_wide && selection.data && (
            <Button onPress={() => setSelecting(true)}>
              {t("kubernetesResources.chooseDashboard")}
            </Button>
          )}
        </div>
      </div>
      {selection.isPending && <Spinner label={t("common.loading")} />}
      {selection.error && (
        <Alert
          tone="danger"
          title={t("dashboards.failed")}
          description={formatApiError(
            selection.error,
            t("dashboards.failed"),
            (id) => t("common.requestReference", { requestId: id }),
          )}
        />
      )}
      {selection.data && !board && (
        <Alert
          tone="warning"
          title={t("kubernetesResources.notAvailable")}
          description={t("kubernetesResources.overviewUnavailable")}
        />
      )}
      {board && (
        <DashboardView
          dashboardId={board.id}
          initialTime={dashboardTimeFromSearch(location.search)}
          onTimeChange={(time) => location.update(dashboardTimeToSearch(time))}
          embedded
          visible={visible}
          entry={{ resource: clusterId, resource_type: "kubernetes_cluster" }}
          fixedScope={fixedScope}
          scopeControls={controls}
          executeOverride={execute}
        />
      )}
      {selecting && selection.data && (
        <OverviewDashboardSelector
          clusterId={clusterId}
          selection={selection.data}
          onClose={() => setSelecting(false)}
        />
      )}
    </section>
  );
}
