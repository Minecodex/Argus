import { useTranslation } from "react-i18next";
import type { PlatformOverview } from "@argus/api-client";
import {
  Card,
  CardContent,
  CardHeader,
  EmptyState,
  MetricChart,
} from "@argus/ui";

/** Recorded monthly usage, shared by the overview and Sandbox usage task. */
export function PlatformUsageChart({
  overview,
}: {
  overview: PlatformOverview;
}) {
  const { t } = useTranslation();
  const usage = overview.monthly_usage;
  return (
    <Card>
      <CardHeader
        title={t("dashboard.usage.title")}
        description={t("dashboard.usage.scope", {
          from: overview.usage_from_month,
          to: overview.usage_to_month,
        })}
      />
      <CardContent>
        {usage.length ? (
          <MetricChart
            labels={usage.map((point) => point.month)}
            series={[
              {
                name: t("dashboard.usage.sessions"),
                points: usage.map((point) => point.session_count),
              },
              {
                name: t("dashboard.usage.minutes"),
                points: usage.map((point) => point.session_seconds / 60),
              },
            ]}
            showLegend
            type="bar"
          />
        ) : (
          <EmptyState title={t("dashboard.usage.empty")} description="" />
        )}
      </CardContent>
    </Card>
  );
}
