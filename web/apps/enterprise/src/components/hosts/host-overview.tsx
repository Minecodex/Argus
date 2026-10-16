import { useState } from "react";
import { usePermission } from "../../lib/permissions";
import { HostReconnectDialog } from "./host-reconnect-dialog";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, formatApiError, type Host } from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  DataTable,
  EmptyState,
  KeyValueGrid,
} from "@argus/ui";
import { RefreshCw } from "lucide-react";
import { formatDateTime } from "./host-utils";
import { HostTelemetrySummary } from "./host-telemetry-summary";
import "../../styles/resource-lists.css";

export function formatInventoryBytes(value: number | undefined): string {
  if (value === undefined) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${Number(value.toFixed(2))} ${units[index]}`;
}

export function HostOverview({ host }: { host: Host }) {
  const { t } = useTranslation(),
    api = useApi(),
    cache = useQueryClient(),
    inventory = host.inventory;
  const canManage = usePermission("host.manage");
  const canTest = usePermission("host.test");
  const canReconnect = canManage && canTest;
  const [reconnectOpen, setReconnectOpen] = useState(false);
  const refresh = useMutation({
    mutationFn: () => api.hosts.refreshInventory(host.id),
    onSuccess: (value) => {
      cache.setQueryData(["hosts", "detail", host.id], value);
      void cache.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
  const unknown = t("hostInventory.unavailable"),
    text = (value: string | undefined) => value || unknown;
  const section = (key: string) => t(`hostInventory.${key}`);
  return (
    <div className="argus-hosts-stack">
      <div className="argus-resource-list__toolbar">
        <h2>{section("title")}</h2>
        <Button
          variant="secondary"
          isPending={refresh.isPending}
          isDisabled={
            host.connection_status !== "online" || host.status !== "active"
          }
          onPress={() => refresh.mutate()}
        >
          <RefreshCw aria-hidden size={16} />
          {section(refresh.isPending ? "refreshing" : "refresh")}
        </Button>
        {canReconnect &&
          host.control_path === "executor_tunnel" &&
          host.connection_status !== "online" &&
          host.status === "active" && (
            <Button onPress={() => setReconnectOpen(true)}>
              {t("hostInventory.reconnect")}
            </Button>
          )}
        {inventory && (
          <span className="argus-muted">
            {section("collected")} · {formatDateTime(inventory.collected_at)}
          </span>
        )}
      </div>
      {refresh.isError && (
        <Alert
          tone="danger"
          title={section("failed")}
          description={formatApiError(
            refresh.error,
            section("failed"),
            (requestId) => t("common.requestReference", { requestId }),
          )}
        />
      )}
      <section className="argus-hosts-stack" aria-label={section("basic")}>
        <h3>{section("basic")}</h3>
        <KeyValueGrid
          columns={3}
          items={[
            {
              label: t("hosts.overview.kv.address"),
              value: host.address ? `${host.address}:${host.port}` : unknown,
            },
            {
              label: t("hosts.overview.kv.environment"),
              value: <Badge>{t(`hosts.env.${host.environment}`)}</Badge>,
            },
            {
              label: t("hosts.overview.kv.connectionMode"),
              value: t(`hosts.controlPath.${host.control_path}`),
            },
            {
              label: t("hosts.overview.kv.lastSeen"),
              value: formatDateTime(host.last_seen_at),
            },
            {
              label: t("hosts.overview.kv.createdAt"),
              value: formatDateTime(host.created_at),
            },
            {
              label: t("hosts.overview.kv.labels"),
              value:
                Object.entries(host.labels)
                  .map(([key, value]) => `${key}=${value}`)
                  .join(", ") || "—",
            },
          ]}
        />
      </section>
      {!inventory ? (
        <EmptyState
          title={section("pending")}
          description={section("pendingHint")}
        />
      ) : (
        <>
          <Alert
            tone="info"
            description=""
            title={section(
              inventory.scope === "container" ? "container" : "machine",
            )}
          />
          {Date.now() - Date.parse(inventory.collected_at) > 600_000 && (
            <Alert tone="warning" description="" title={section("stale")} />
          )}
          {inventory.issues.length > 0 && (
            <Alert
              tone="warning"
              description=""
              title={t("hostInventory.partial", {
                sections: [
                  ...new Set(
                    inventory.issues.map((issue) =>
                      section(
                        issue.section === "system"
                          ? "system"
                          : issue.section === "machine_id"
                            ? "machineId"
                            : issue.section === "hardware_uuid"
                              ? "hardwareUuid"
                              : issue.section,
                      ),
                    ),
                  ),
                ].join(", "),
              })}
            />
          )}
          <KeyValueGrid
            columns={3}
            items={[
              { label: section("hostname"), value: text(inventory.hostname) },
              {
                label: section("system"),
                value: text(`${inventory.os} ${inventory.os_version}`.trim()),
              },
              {
                label: section("kernel"),
                value: text(inventory.kernel_version),
              },
              {
                label: section("architecture"),
                value: text(inventory.architecture),
              },
              {
                label: section("machineId"),
                value: text(inventory.machine_id),
              },
              {
                label: section("hardwareUuid"),
                value: text(inventory.hardware_uuid),
              },
              {
                label: section("virtualization"),
                value: text(
                  [
                    inventory.virtualization_system,
                    inventory.virtualization_role,
                  ]
                    .filter(Boolean)
                    .join(" / "),
                ),
              },
              {
                label: section("boot"),
                value: inventory.boot_time
                  ? formatDateTime(inventory.boot_time)
                  : unknown,
              },
              { label: section("cpuModel"), value: text(inventory.cpu?.model) },
              {
                label: section("logicalCores"),
                value: inventory.cpu?.logical_cores || unknown,
              },
              {
                label: section("physicalCores"),
                value: inventory.cpu?.physical_cores || unknown,
              },
              {
                label: section("totalMemory"),
                value: inventory.memory
                  ? formatInventoryBytes(inventory.memory.total_bytes)
                  : unknown,
              },
              {
                label: section("swap"),
                value: inventory.memory
                  ? formatInventoryBytes(inventory.memory.swap_bytes)
                  : unknown,
              },
              {
                label: section("provider"),
                value:
                  inventory.cloud.status === "available"
                    ? text(inventory.cloud.provider)
                    : section("notDetected"),
              },
              {
                label: section("instanceId"),
                value: text(inventory.cloud.instance_id),
              },
              { label: section("region"), value: text(inventory.cloud.region) },
              { label: section("zone"), value: text(inventory.cloud.zone) },
            ]}
          />
          <section>
            <h3>{section("disks")}</h3>
            <DataTable
              data={inventory.disks}
              getRowKey={(disk) => disk.mountpoint}
              columns={[
                { key: "device", header: section("device") },
                { key: "mountpoint", header: section("mount") },
                { key: "filesystem", header: section("filesystem") },
                {
                  key: "total_bytes",
                  header: section("total"),
                  render: (disk) => formatInventoryBytes(disk.total_bytes),
                },
                {
                  key: "used_bytes",
                  header: section("used"),
                  render: (disk) => formatInventoryBytes(disk.used_bytes),
                },
                {
                  key: "free_bytes",
                  header: section("free"),
                  render: (disk) => formatInventoryBytes(disk.free_bytes),
                },
              ]}
            />
            {!inventory.disks.length && (
              <p className="argus-muted">{unknown}</p>
            )}
          </section>
          <section>
            <h3>{section("networks")}</h3>
            <DataTable
              data={inventory.networks}
              getRowKey={(network) => network.name}
              columns={[
                { key: "name", header: section("name") },
                {
                  key: "state",
                  header: section("state"),
                  render: (network) => section(network.state),
                },
                {
                  key: "addresses",
                  header: section("addresses"),
                  render: (network) => network.addresses.join(", ") || unknown,
                },
                {
                  key: "mac",
                  header: section("mac"),
                  render: (network) => text(network.mac),
                },
                { key: "mtu", header: section("mtu") },
              ]}
            />
          </section>
          <section>
            <h3>{section("gpus")}</h3>
            {inventory.gpus.length ? (
              <DataTable
                data={inventory.gpus}
                getRowKey={(gpu) => gpu.device_id}
                columns={[
                  { key: "name", header: section("name") },
                  { key: "vendor", header: section("vendor") },
                  { key: "device_id", header: section("deviceId") },
                  {
                    key: "memory_bytes",
                    header: section("videoMemory"),
                    render: (gpu) =>
                      gpu.memory_bytes
                        ? formatInventoryBytes(gpu.memory_bytes)
                        : unknown,
                  },
                ]}
              />
            ) : (
              <p className="argus-muted">
                {inventory.issues.some((issue) => issue.section === "gpus")
                  ? unknown
                  : section("notDetected")}
              </p>
            )}
          </section>
        </>
      )}
      <HostTelemetrySummary host={host} />
      {reconnectOpen && (
        <HostReconnectDialog
          host={host}
          onClose={() => setReconnectOpen(false)}
        />
      )}
    </div>
  );
}
