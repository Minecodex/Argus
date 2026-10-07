import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, ApiError, type KubernetesCluster } from "@argus/api-client";
import {
  ActionGroup,
  Badge,
  Button,
  ResourceCard,
  RowAction,
  StatusBadge,
} from "@argus/ui";
import {
  bindingCoverage,
  collectorStatusTone,
  connectionStatusTone,
} from "./status";
export function ClusterCard({
  cluster,
  onOpen,
  onEdit,
  onDelete,
  onInstallCollector,
  onOpenCollector,
}: {
  cluster: KubernetesCluster;
  onOpen: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onInstallCollector?: () => void;
  onOpenCollector?: () => void;
}) {
  const { t } = useTranslation(),
    api = useApi(),
    showCollector = Boolean(onInstallCollector || onOpenCollector);
  const collector = useQuery({
    queryKey: ["kubernetes", "collector", cluster.id],
    queryFn: () => api.kubernetes.getCollector(cluster.id),
    enabled: showCollector,
  });
  const bindings = useQuery({
    queryKey: ["kubernetes", "nodeBindings", cluster.id],
    queryFn: () => api.kubernetes.listNodeBindings(cluster.id),
    enabled: showCollector,
  });
  const state =
      collector.data?.status ??
      ((collector.isSuccess && collector.data === null) ||
      (collector.error instanceof ApiError && collector.error.status === 404)
        ? "not_installed"
        : undefined),
    coverage = bindings.data
      ? bindingCoverage(cluster, bindings.data)
      : undefined;
  const unavailable = t("kubernetes.card.dataUnavailable"),
    loading = t("common.loading");
  return (
    <ResourceCard
      title={cluster.name}
      subtitle={cluster.api_server}
      onOpen={onOpen}
      status={
        <StatusBadge tone={connectionStatusTone(cluster.connection_status)}>
          {t(`kubernetes.status.${cluster.connection_status}`)}
        </StatusBadge>
      }
      labels={
        <Badge tone="accent">
          {t(`kubernetes.environment.${cluster.environment}`)}
        </Badge>
      }
      facts={[
        {
          label: t("kubernetes.card.version"),
          value: cluster.kubernetes_version || "—",
        },
        {
          label: t("kubernetes.card.nodes"),
          value: `${cluster.ready_node_count}/${cluster.node_count}`,
        },
        ...(showCollector
          ? [
              {
                label: t("kubernetes.card.collector"),
                value: state ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={t(
                      state === "not_installed"
                        ? "kubernetes.card.installCollector"
                        : "kubernetes.card.openCollector",
                      { name: cluster.name },
                    )}
                    onPress={
                      state === "not_installed"
                        ? onInstallCollector
                        : onOpenCollector
                    }
                  >
                    <StatusBadge tone={collectorStatusTone(state)}>
                      {t(`kubernetes.collectorStatus.${state}`)}
                    </StatusBadge>
                  </Button>
                ) : collector.isPending ? (
                  loading
                ) : (
                  unavailable
                ),
              },
              {
                label: t("kubernetes.card.bindingCoverage"),
                value: coverage
                  ? `${coverage.verified}/${coverage.total} (${coverage.percent}%)`
                  : bindings.isPending
                    ? loading
                    : unavailable,
              },
            ]
          : []),
      ]}
      actions={
        <ActionGroup>
          <RowAction onPress={onOpen}>{t("kubernetes.card.open")}</RowAction>
        </ActionGroup>
      }
      menuItems={[
        { label: t("kubernetes.card.edit"), onSelect: onEdit },
        {
          label: t("kubernetes.card.delete"),
          onSelect: onDelete,
          danger: true,
        },
      ]}
    />
  );
}
