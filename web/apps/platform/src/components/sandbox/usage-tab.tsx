import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { QueryBoundary, StatCard } from "@argus/ui";
import { PlatformUsageChart } from "../platform-usage-chart";

/** Complete recorded monthly usage; unsupported measures remain unknown. */
export function UsageTab() {
  const { t, i18n } = useTranslation();
  const api = useApi();

  const overview = useQuery({
    queryKey: ["platform", "overview"],
    queryFn: ({ signal }) => api.platform.overview.get(signal),
  });

  const totals = useMemo(
    () =>
      (overview.data?.monthly_usage ?? []).reduce(
        (sum, point) => ({
          sessions: sum.sessions + point.session_count,
          seconds: sum.seconds + point.session_seconds,
        }),
        { sessions: 0, seconds: 0 },
      ),
    [overview.data],
  );

  return (
    <div className="argus-platform-stack">
      <QueryBoundary query={overview}>
        {overview.data && (
          <>
            <div className="argus-stat-row">
              <StatCard
                label={t("sandbox.usage.totalSessions")}
                tone="accent"
                value={totals.sessions}
              />
              <StatCard
                label={t("sandbox.usage.totalMinutes")}
                value={(totals.seconds / 60).toLocaleString(i18n.language, {
                  maximumFractionDigits: 2,
                })}
              />
              <StatCard
                label={t("sandbox.usage.totalCpu")}
                value="—"
                detail={t("sandbox.usage.cpuUnavailable")}
              />
              <StatCard
                label={t("sandbox.usage.activeNow")}
                tone="info"
                value={overview.data.active_sandbox_session_count}
              />
            </div>

            <PlatformUsageChart overview={overview.data} />
          </>
        )}
      </QueryBoundary>
    </div>
  );
}
