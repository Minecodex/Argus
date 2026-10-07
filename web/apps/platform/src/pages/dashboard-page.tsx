import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  auditPresentationKey,
  humanizeAuditCode,
  useApi,
} from "@argus/api-client";
import type { PlatformAuditEvent } from "@argus/api-client";
import {
  Card,
  CardContent,
  CardHeader,
  DataTable,
  EmptyState,
  PageShell,
  QueryBoundary,
  StatCard,
  StatusBadge,
} from "@argus/ui";
import { formatDateTime } from "../lib/format";
import { PlatformUsageChart } from "../components/platform-usage-chart";

type AuditRow = {
  id: string;
  createdAt: string;
  actorName: string;
  actionLabel: string;
  summary: string;
  result: PlatformAuditEvent["result"];
};

function resultTone(result: PlatformAuditEvent["result"]) {
  if (result === "success") return "success" as const;
  if (result === "denied") return "warning" as const;
  return "danger" as const;
}

/** 仪表盘：平台级统计 + 用量趋势 + 最近平台审计事件。 */
export function DashboardPage() {
  const { t, i18n } = useTranslation();
  const api = useApi();
  const actionLabel = (item: PlatformAuditEvent) =>
    t(auditPresentationKey("audit", "actions", item.action), {
      defaultValue: humanizeAuditCode(item.action),
    });

  const overview = useQuery({
    queryKey: ["platform", "overview"],
    queryFn: ({ signal }) => api.platform.overview.get(signal),
  });
  const audit = useQuery({
    queryKey: ["platform", "audit", "recent"],
    queryFn: () => api.platform.audit.list(),
  });

  const recentEvents: AuditRow[] = (audit.data?.items ?? [])
    .slice(0, 8)
    .map((item) => ({
      id: item.id,
      createdAt: item.createdAt,
      actorName: item.actorName,
      actionLabel: actionLabel(item),
      summary: item.resourceName
        ? t("audit.summaryWithResource", {
            action: actionLabel(item),
            resource: item.resourceName,
          })
        : actionLabel(item),
      result: item.result,
    }));

  return (
    <PageShell
      description={t("dashboard.description")}
      title={t("dashboard.title")}
    >
      <div className="argus-platform-stack">
        <QueryBoundary query={overview}>
          {overview.data && (
            <>
              <div className="argus-stat-row">
                <StatCard
                  label={t("dashboard.stats.enterprises")}
                  tone="accent"
                  value={overview.data.enterprise_count}
                />
                <StatCard
                  label={t("dashboard.stats.activeEnterprises")}
                  tone="success"
                  value={overview.data.active_enterprise_count}
                />
                <StatCard
                  label={t("dashboard.stats.activeSessions")}
                  tone="info"
                  value={overview.data.active_sandbox_session_count}
                />
                <StatCard
                  detail={t("dashboard.pendingDetail")}
                  label={t("dashboard.stats.pending")}
                  tone={
                    overview.data.pending_admin_count > 0
                      ? "warning"
                      : "neutral"
                  }
                  value={overview.data.pending_admin_count}
                />
              </div>

              <PlatformUsageChart overview={overview.data} />
            </>
          )}
        </QueryBoundary>

        <Card>
          <CardHeader title={t("dashboard.recent.title")} />
          <CardContent>
            <QueryBoundary query={audit}>
              {recentEvents.length === 0 ? (
                <EmptyState
                  description=""
                  title={t("dashboard.recent.empty")}
                />
              ) : (
                <DataTable<AuditRow>
                  columns={[
                    {
                      key: "createdAt",
                      header: t("dashboard.recent.time"),
                      render: (row) =>
                        formatDateTime(row.createdAt, i18n.language),
                    },
                    { key: "actorName", header: t("dashboard.recent.actor") },
                    {
                      key: "actionLabel",
                      header: t("dashboard.recent.action"),
                    },
                    { key: "summary", header: t("dashboard.recent.summary") },
                    {
                      key: "result",
                      header: t("dashboard.recent.result"),
                      render: (row) => (
                        <StatusBadge tone={resultTone(row.result)}>
                          {t(`audit.results.${row.result}`)}
                        </StatusBadge>
                      ),
                    },
                  ]}
                  data={recentEvents}
                  getRowKey={(row) => row.id}
                />
              )}
            </QueryBoundary>
          </CardContent>
        </Card>
      </div>
    </PageShell>
  );
}
