import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type KubernetesCluster } from "@argus/api-client";
import {
  ActionMenu,
  Badge,
  DataTable,
  StatusBadge,
} from "@argus/ui";
import { collectorStatusTone, connectionStatusTone } from "./status";
import { usePermission } from "../../lib/permissions";

export function ClusterList({
  clusters,
  onEdit,
  onDelete,
  onOpenCollector,
}: {
  clusters: KubernetesCluster[];
  onEdit: (cluster: KubernetesCluster) => void;
  onDelete: (cluster: KubernetesCluster) => void;
  onOpenCollector: (cluster: KubernetesCluster) => void;
}) {
  const { t } = useTranslation(),
    api = useApi(),
    canRead = usePermission("telemetry.collector.read");
  const collectors = useQuery({
    queryKey: ["telemetry", "collectors"],
    queryFn: () => api.telemetry.listCollectors(),
    enabled: canRead,
    refetchInterval: 15_000,
  });
  return (
    <DataTable
      data={clusters}
      getRowKey={(cluster) => cluster.id}
      columns={[
        {
          key: "name",
          header: t("resourceList.name"),
          render: (cluster) => (
            <div className="argus-resource-list__identity">
              <Link
                to="/kubernetes/$clusterId"
                params={{ clusterId: cluster.id }}
              >
                {cluster.name}
              </Link>
              <span className="argus-muted argus-mono">
                {cluster.api_server}
              </span>
            </div>
          ),
        },
        {
          key: "connection_status",
          header: t("resourceList.status"),
          render: (cluster) => (
            <StatusBadge tone={connectionStatusTone(cluster.connection_status)}>
              {t(`kubernetes.status.${cluster.connection_status}`)}
            </StatusBadge>
          ),
        },
        {
          key: "environment",
          header: t("resourceList.environment"),
          render: (cluster) => (
            <Badge>{t(`kubernetes.environment.${cluster.environment}`)}</Badge>
          ),
        },
        {
          key: "kubernetes_version",
          header: t("resourceList.version"),
          render: (cluster) => cluster.kubernetes_version || "—",
        },
        {
          key: "node_count",
          header: t("resourceList.nodes"),
          render: (cluster) =>
            cluster.node_count === undefined ||
            cluster.ready_node_count === undefined
              ? t("resourceList.unavailable")
              : `${cluster.ready_node_count} / ${cluster.node_count}`,
        },
        {
          key: "collector",
          header: t("resourceList.collector"),
          render: (cluster) => {
            if (!canRead || collectors.isError)
              return t("resourceList.unavailable");
            if (!collectors.isSuccess) return t("common.loading");
            const state =
              collectors.data.find((entry) => entry.resource_id === cluster.id)
                ?.status ?? "not_installed";
            return (
                <StatusBadge tone={collectorStatusTone(state)}>
                  {t(`kubernetes.collectorStatus.${state}`)}
                </StatusBadge>
            );
          },
        },
        {
          key: "actions",
          header: t("resourceList.actions"),
          align: "right",
          render: (cluster) => (
              <ActionMenu
                name={cluster.name}
                items={[
                  {
                    label: t("kubernetes.detail.collectorTab"),
                    onSelect: () => onOpenCollector(cluster),
                  },
                  {
                    label: t("kubernetes.card.edit"),
                    onSelect: () => onEdit(cluster),
                  },
                  {
                    label: t("kubernetes.card.delete"),
                    onSelect: () => onDelete(cluster),
                    danger: true,
                  },
                ]}
              />
          ),
        },
      ]}
    />
  );
}
