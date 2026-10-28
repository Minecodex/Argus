import type { ReactNode } from "react";
import type { AlertFilter } from "@argus/api-client";
import { FilterBar, type FilterBarFilter } from "@argus/ui";
import { useAlertText } from "./common";

export function AlertFilters({
  tab,
  value,
  onChange,
  actions,
}: {
  tab: string;
  value: AlertFilter;
  onChange: (value: Partial<AlertFilter>) => void;
  actions?: ReactNode;
}) {
  const { t } = useAlertText();
  const filters: FilterBarFilter[] = [];
  const options = (values: string[]) =>
    values.map((value) => ({ value, label: t(`alerts.${value}`) }));
  if (tab === "active" || tab === "history")
    filters.push({
      key: "severity",
      value: value.severity ?? "",
      allLabel: t("alerts.allLevels"),
      options: options(["critical", "warning", "info"]),
      onChange: (severity) => onChange({ severity }),
    });
  if (tab === "active")
    filters.push({
      key: "acknowledged",
      value: value.acknowledged === undefined ? "" : String(value.acknowledged),
      allLabel: t("alerts.allHandling"),
      options: [
        { value: "false", label: t("alerts.unacknowledged") },
        { value: "true", label: t("alerts.acknowledged") },
      ],
      onChange: (acknowledged) =>
        onChange({
          acknowledged:
            acknowledged === "" ? undefined : acknowledged === "true",
        }),
    });
  if (tab === "history")
    filters.push({
      key: "lifecycle",
      value: value.lifecycle ?? "",
      allLabel: t("alerts.allLifecycle"),
      options: options([
        "firing",
        "pending",
        "recovering",
        "resolved",
        "closed",
      ]),
      onChange: (lifecycle) => onChange({ lifecycle }),
    });
  if (tab === "deliveries")
    filters.push({
      key: "notification_state",
      value: value.notification_state ?? "",
      allLabel: t("alerts.allStates"),
      ariaLabel: t("alerts.notification"),
      options: options([
        "pending",
        "sending",
        "accepted",
        "failed",
        "unknown",
        "muted",
        "inhibited",
        "canceled",
      ]),
      onChange: (notification_state) => onChange({ notification_state }),
    });
  return (
    <FilterBar
      className="argus-alert-filter-bar"
      search={{
        value: value.query ?? "",
        placeholder: t("alerts.search"),
        onChange: (query) => onChange({ query }),
      }}
      filters={filters}
      dateRange={
        tab === "history"
          ? {
              from: value.from?.slice(0, 10) ?? "",
              to: value.to?.slice(0, 10) ?? "",
              fromLabel: t("alerts.fromDate"),
              toLabel: t("alerts.toDate"),
              onFromChange: (from) =>
                onChange({ from: from ? `${from}T00:00:00+08:00` : undefined }),
              onToChange: (to) =>
                onChange({ to: to ? `${to}T23:59:59+08:00` : undefined }),
            }
          : undefined
      }
      actions={actions}
    />
  );
}
