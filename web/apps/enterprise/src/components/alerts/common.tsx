import { useTranslation } from "react-i18next";
import {
  StatusBadge,
  Button,
  DataTable,
  Pagination,
  type Column,
} from "@argus/ui";
import type {
  AlertInstance,
  AlertConfiguration,
  AlertSchemas,
} from "@argus/api-client";
import type { ReactNode } from "react";
import { useState } from "react";
import { Info } from "lucide-react";
export function useAlertText() {
  const { t: translate, i18n } = useTranslation();
  return {
    t: (key: string, vars?: Record<string, unknown>) =>
      String(translate(key, vars)),
    locale: i18n.language,
  };
}
export function AlertStatus({
  value,
  tone: explicitTone,
}: {
  value: string;
  tone?: "danger" | "warning" | "success" | "info" | "accent" | "neutral";
}) {
  const { t } = useAlertText();
  const tone: [
    string[],
    "danger" | "warning" | "success" | "info" | "accent",
  ][] = [
    [["critical", "failed", "error", "authorization_denied"], "danger"],
    [["warning", "no_data", "unknown", "pending", "not_evaluated"], "warning"],
    [["accepted", "resolved", "normal", "enabled", "success"], "success"],
    [["inhibited", "muted", "info"], "info"],
    [["firing", "recovering", "model"], "accent"],
  ];
  return (
    <StatusBadge
      appearance="flat"
      className="argus-alert-badge"
      tone={
        explicitTone ??
        tone.find(([keys]) => keys.includes(value))?.[1] ??
        "neutral"
      }
    >
      {t(value === "not_evaluated" ? "alerts.notEvaluated" : `alerts.${value}`)}
    </StatusBadge>
  );
}
export function AlertTable<T extends object>({
  data,
  columns,
  empty = true,
  footer,
  framed = true,
}: {
  data: T[];
  columns: Column<T>[];
  empty?: boolean;
  footer?: ReactNode;
  framed?: boolean;
}) {
  const { t } = useAlertText();
  const [localPage, setLocalPage] = useState(1);
  const pages = Math.max(1, Math.ceil(data.length / 5));
  const page = Math.min(localPage, pages);
  const displayed =
    footer === undefined ? data.slice((page - 1) * 5, page * 5) : data;
  return (
    <div
      className={framed ? "argus-alert-surface" : "argus-alert-table-layout"}
    >
      <DataTable
        density="compact"
        surface={framed ? "plain" : "outlined"}
        tableClassName="argus-alert-table"
        data={displayed as (T & Record<string, unknown>)[]}
        columns={columns as Column<T & Record<string, unknown>>[]}
        getRowKey={(i) => String(i["id"] ?? "")}
      />
      {empty && !data.length && (
        <div className="argus-alert-empty">
          {t("alerts.empty")}
          <p>{t("alerts.emptyHelp")}</p>
        </div>
      )}
      {footer === undefined ? (
        <Pagination
          page={page}
          totalPages={pages}
          onChange={setLocalPage}
          summary={t("alerts.pageInfo", { total: data.length, page, pages })}
        />
      ) : (
        footer
      )}
    </div>
  );
}
export function SmallAction({
  children,
  onPress,
  disabled,
  primary = false,
}: {
  children: ReactNode;
  onPress: () => void;
  disabled?: boolean;
  primary?: boolean;
}) {
  return (
    <Button
      density="compact"
      variant={primary ? "primary" : "ghost"}
      size="sm"
      isDisabled={disabled}
      onPress={onPress}
    >
      {children}
    </Button>
  );
}
export function AlertCallout({
  children,
  danger = false,
}: {
  children: ReactNode;
  danger?: boolean;
}) {
  return (
    <div
      role={danger ? "alert" : "note"}
      className={`argus-alert-callout${danger ? " argus-alert-callout--danger" : ""}`}
    >
      <Info size={16} className="argus-alert-icon" aria-hidden />
      {children}
    </div>
  );
}
export const policySpec = (c: AlertConfiguration) =>
  c.spec as AlertSchemas["AlertPolicySpec"];
export function resourceName(e: AlertInstance, names: Map<string, string>) {
  return (
    e.state.labels["resource_name"] ??
    e.resource_ids.map((id) => names.get(id) ?? id).join(", ")
  );
}
export function clock(value?: string, seconds = false) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: seconds ? "2-digit" : undefined,
    timeZone: "Asia/Shanghai",
  }).format(new Date(value));
}
export function datetime(value?: string) {
  if (!value) return "—";
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).formatToParts(new Date(value));
  const part = (type: string) =>
    parts.find((entry) => entry.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")} ${part("hour")}:${part("minute")}:${part("second")}`;
}
export function duration(
  e: AlertInstance,
  t: (key: string, vars?: Record<string, unknown>) => string,
  anchor?: string,
) {
  if (!e.started_at) return "—";
  const end = new Date(
    e.resolved_at ?? anchor ?? e.last_evaluated_at,
  ).getTime();
  const seconds = Math.max(
    0,
    Math.round((end - new Date(e.started_at).getTime()) / 1000),
  );
  if (seconds < 60) return t("alerts.secondValue", { n: seconds });
  return t("alerts.minuteValue", {
    n: Math.max(
      0,
      Math.round((end - new Date(e.started_at).getTime()) / 60000),
    ),
  });
}
