import type { DashboardSchemas } from "@argus/api-client";
export type DashboardTimeSearch = {
  time_seconds?: number;
  time_from?: string;
  time_to?: string;
};
export function parseDashboardTime(
  search: Record<string, unknown>,
): DashboardTimeSearch {
  const seconds = Number(search.time_seconds);
  const from =
      typeof search.time_from === "string" ? search.time_from : undefined,
    to = typeof search.time_to === "string" ? search.time_to : undefined;
  if (
    from &&
    to &&
    Number.isFinite(Date.parse(from)) &&
    Number.isFinite(Date.parse(to)) &&
    Date.parse(to) > Date.parse(from) &&
    Date.parse(to) - Date.parse(from) <= 2678400000
  )
    return { time_from: from, time_to: to };
  return Number.isInteger(seconds) && seconds >= 60 && seconds <= 2678400
    ? { time_seconds: seconds }
    : {};
}
export function dashboardTimeFromSearch(
  search: DashboardTimeSearch,
): DashboardSchemas["DashboardTimeRange"] | undefined {
  if (search.time_from && search.time_to)
    return { kind: "absolute", from: search.time_from, to: search.time_to };
  if (search.time_seconds)
    return { kind: "relative", seconds: search.time_seconds };
}
export function dashboardTimeToSearch(
  value: DashboardSchemas["DashboardTimeRange"],
): DashboardTimeSearch {
  return value.kind === "absolute"
    ? { time_from: value.from, time_to: value.to, time_seconds: undefined }
    : { time_from: undefined, time_to: undefined, time_seconds: value.seconds };
}
