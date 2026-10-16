import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type Host } from "@argus/api-client";
import { DataTable } from "@argus/ui";
import { usePermission } from "../../lib/permissions";

const metrics = [
  { id: "cpu", query: '100 * (1 - avg(system_cpu_utilization{state="idle"}))' },
  { id: "memory", query: '100 * avg(system_memory_utilization{state="used"})' },
  {
    id: "disks",
    query:
      'max(100 * system_filesystem_usage{state="used"} / ignoring(state) group_left sum without(state) (system_filesystem_usage))',
  },
];
export function HostTelemetrySummary({ host }: { host: Host }) {
  const api = useApi(),
    { t } = useTranslation(),
    canRead = usePermission("host.read");
  const query = useQuery({
    queryKey: ["host-telemetry-summary", host.id],
    enabled: canRead,
    refetchInterval: 60_000,
    queryFn: async () => {
      const end = new Date(),
        from = new Date(end.getTime() - 300_000);
      return Promise.all(
        metrics.map(async (metric) => {
          try {
            const response = await api.telemetry.queryMetrics({
              query: metric.query,
              resource_ids: [host.id],
              time_range: { from: from.toISOString(), to: end.toISOString() },
            });
            const result = response.data.result as {
              value?: [number, string];
            }[];
            const raw = Array.isArray(result)
              ? result[0]?.value?.[1]
              : undefined;
            const value =
              raw !== undefined && Number.isFinite(Number(raw))
                ? Number(raw)
                : undefined;
            return { id: metric.id, value, failed: false };
          } catch {
            return { id: metric.id, value: undefined, failed: true };
          }
        }),
      );
    },
  });
  if (!canRead)
    return <p className="argus-muted">{t("hostInventory.telemetryDenied")}</p>;
  const rows =
    query.data ??
    metrics.map((metric) => ({
      id: metric.id,
      value: undefined,
      failed: false,
    }));
  return (
    <section>
      <h3>{t("hostInventory.summary")}</h3>
      <p className="argus-muted">{t("hostInventory.metricHint")}</p>
      <DataTable
        data={rows}
        getRowKey={(row) => row.id}
        columns={[
          {
            key: "id",
            header: t("resourceList.name"),
            render: (row) => t(`hostInventory.${row.id}`),
          },
          {
            key: "value",
            header: t("hostInventory.usage"),
            render: (row) =>
              query.isPending
                ? t("common.loading")
                : row.failed
                  ? t("hostInventory.telemetryFailed")
                  : row.value === undefined
                    ? t("hostInventory.noSample")
                    : `${Number(row.value.toFixed(2))}%`,
          },
        ]}
      />
    </section>
  );
}
