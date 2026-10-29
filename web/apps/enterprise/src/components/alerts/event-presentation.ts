import type { AlertInstance } from "@argus/api-client";

type Translate = (key: string) => string;

export function queryHealthCause(event: AlertInstance): string {
  return event.state.labels["alert_health"] ?? event.health;
}

export function alertEventTitle(event: AlertInstance, t: Translate): string {
  if (event.state.delivery_gap) return t("alerts.deliveryGapTitle");
  if (event.state.kind !== "health") return event.state.name;
  const cause = queryHealthCause(event);
  if (cause === "no_data") return t("alerts.queryHealthNoDataTitle");
  if (cause === "error") return t("alerts.queryHealthErrorTitle");
  if (cause === "authorization_denied")
    return t("alerts.queryHealthDeniedTitle");
  return t("alerts.queryHealthTitle");
}
