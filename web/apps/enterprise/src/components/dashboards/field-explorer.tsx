import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type DashboardSchemas,
  type DashboardSpec,
} from "@argus/api-client";
import { TelemetryFieldExplorer } from "@argus/ui";

export function DashboardFieldExplorer({
  signal,
  source,
  time,
  metric,
  onUse,
  canUse,
}: {
  signal: "metrics" | "logs" | "traces";
  source: DashboardSchemas["DashboardSourceBinding"];
  time: DashboardSpec["default_time_range"];
  metric?: string;
  onUse?: (field: string, value: string) => void;
  canUse?: (field: string) => boolean;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation();
  // Freeze relative time across all pages. The parent keys this component by
  // source, metric and authored time so stale pages cannot cross scope changes.
  const [range] = useState(() => {
    const to = time.kind === "absolute" ? new Date(time.to!) : new Date();
    const from =
      time.kind === "absolute"
        ? new Date(time.from!)
        : new Date(to.getTime() - (time.seconds ?? 3600) * 1000);
    return { from: from.toISOString(), to: to.toISOString() };
  });
  return (
    <TelemetryFieldExplorer
      scope={t("dashboards.fieldScope", {
        source: source.source_type,
        from: new Date(range.from).toLocaleString(i18n.language),
        to: new Date(range.to).toLocaleString(i18n.language),
      })}
      onUse={onUse}
      canUse={canUse}
      load={(request, cancel) =>
        api.dashboards.catalog(
          {
            ...range,
            ...request,
            signal,
            source_binding: source,
            metric,
            filters: [],
            resource_ids: [],
            selected_values: [],
            limit: 25,
          },
          cancel,
        )
      }
    />
  );
}
