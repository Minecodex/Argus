import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type Host, type BastionScope } from "@argus/api-client";
import { Badge, DataTable, StatusBadge } from "@argus/ui";
import { HostRowActions } from "./host-row-actions";
import { HostOnboardingProgress } from "./host-onboarding-progress";
import {
  collectorTone,
  connectionPathKey,
  environmentTone,
  formatDateTime,
  hostConnectionPresentation,
  scopeOf,
} from "./host-utils";
import { usePermission } from "../../lib/permissions";

export function HostList({
  hosts,
  scopes,
  onEdit,
  onDelete,
  onRemove,
  onCollectorAction,
}: {
  hosts: Host[];
  scopes: BastionScope[];
  onEdit: (host: Host) => void;
  onDelete: (host: Host) => void;
  onRemove: (host: Host) => void;
  onCollectorAction: (host: Host) => void;
}) {
  const { t } = useTranslation(),
    api = useApi();
  const canRead = usePermission("telemetry.collector.read");
  const collectors = useQuery({
    queryKey: ["telemetry", "collectors"],
    queryFn: () => api.telemetry.listCollectors(),
    enabled: canRead,
    refetchInterval: 15_000,
  });
  return (
    <DataTable
      data={hosts}
      getRowKey={(host) => host.id}
      surface="plain"
      columns={[
        {
          key: "name",
          header: t("resourceList.name"),
          render: (host) => (
            <div className="argus-resource-list__identity">
              <Link to="/hosts/$hostId" params={{ hostId: host.id }}>
                {host.name}
              </Link>
              <span className="argus-muted">{host.hostname || "—"}</span>
            </div>
          ),
        },
        {
          key: "address",
          header: t("resourceList.address"),
          render: (host) => (
            <span className="argus-mono">
              {host.address ? `${host.address}:${host.port}` : "—"}
            </span>
          ),
        },
        {
          key: "connection_status",
          header: t("resourceList.status"),
          render: (host) => (
            <div className="argus-resource-list__identity">
              <StatusBadge
                tone={hostConnectionPresentation(host).tone}
                pulse={hostConnectionPresentation(host).pulse}
              >
                {t(hostConnectionPresentation(host).key)}
              </StatusBadge>
              {host.status !== "active" && (
                <Badge tone="warning">
                  {t(`hosts.removal.resourceStatus.${host.status}`)}
                </Badge>
              )}
              <HostOnboardingProgress host={host} showActions={false} />
            </div>
          ),
        },
        {
          key: "environment",
          header: t("resourceList.environment"),
          render: (host) => (
            <Badge tone={environmentTone(host.environment)}>
              {t(`hosts.env.${host.environment}`)}
            </Badge>
          ),
        },
        {
          key: "platform",
          header: t("resourceList.system"),
          render: (host) =>
            `${host.platform === "linux" ? "Linux" : "Windows"} / ${host.architecture || "—"}`,
        },
        {
          key: "control_path",
          header: t("resourceList.path"),
          render: (host) =>
            t(`hosts.path.${connectionPathKey(host)}`, {
              scope: scopeOf(host, scopes)?.name ?? host.bastion_scope_id ?? "",
              address: `${host.address}:${host.port}`,
            }),
        },
        {
          key: "collector",
          header: t("resourceList.collector"),
          render: (host) => {
            if (!canRead) return t("resourceList.unavailable");
            if (!collectors.isSuccess)
              return collectors.isPending
                ? t("common.loading")
                : t("resourceList.unavailable");
            const status =
              collectors.data.find((entry) => entry.resource_id === host.id)
                ?.status ?? "not_installed";
            return (
              <StatusBadge tone={collectorTone(status)}>
                {t(`hosts.collectorStatus.${status}`)}
              </StatusBadge>
            );
          },
        },
        {
          key: "last_seen_at",
          header: t("resourceList.lastSeen"),
          render: (host) => formatDateTime(host.last_seen_at),
        },
        {
          key: "actions",
          header: t("resourceList.actions"),
          align: "right",
          render: (host) => (
            <HostRowActions
              host={host}
              hasCollector={Boolean(
                canRead &&
                collectors.data?.some(
                  (entry) =>
                    entry.resource_id === host.id &&
                    [
                      "converged",
                      "pending_config",
                      "degraded",
                      "backlog",
                    ].includes(entry.status),
                ),
              )}
              onEdit={onEdit}
              onDelete={onDelete}
              onRemove={onRemove}
              onCollectorAction={onCollectorAction}
            />
          ),
        },
      ]}
    />
  );
}
