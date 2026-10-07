import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type DashboardSchemas,
  type DashboardSpec,
} from "@argus/api-client";
import { TelemetryFieldExplorer } from "@argus/ui";
import { useDashboardQueryScope, queryScopeBounds } from "./query-scope";

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
  const scope = useDashboardQueryScope();
  const api = useApi(),
    { t, i18n } = useTranslation();
  // Freeze relative time across all pages. The parent keys this component by
  // source, metric and authored time so stale pages cannot cross scope changes.
  const { kind, from, to, seconds } = scope?.time ?? time;
  const range = useMemo(
    () => queryScopeBounds({ kind, from, to, seconds }),
    [kind, from, to, seconds],
  );
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
            resource_ids: scope?.resources ?? [],
            selected_values: [],
            limit: 25,
          },
          cancel,
        )
      }
    />
  );
}
