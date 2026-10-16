import type { components } from "./generated/alertsapi";
import type { PendingActionPublic } from "./types";
export type AlertSchemas = components["schemas"];
export type AlertKind = AlertSchemas["AlertKind"];
export type AlertConfiguration = AlertSchemas["AlertConfiguration"];
export type AlertPolicySpec = AlertSchemas["AlertPolicySpec"];
export type AlertRule = AlertSchemas["AlertRule"];
export type AlertMutation = AlertSchemas["AlertMutation"];
export type AlertInstance = AlertSchemas["AlertInstance"];
export type AlertDelivery = AlertSchemas["AlertDelivery"];
export type AlertCommand = AlertSchemas["AlertCommand"];
export type AlertFilter = {
  query?: string;
  severity?: string;
  lifecycle?: string;
  notification_state?: string;
  policy_id?: string;
  channel_id?: string;
  rule_id?: string;
  resource_id?: string;
  metric?: string;
  from?: string;
  to?: string;
  acknowledged?: boolean;
  limit?: number;
  cursor?: string;
};
export interface AlertDomains {
  alerts: {
    summary(): Promise<AlertSchemas["AlertSummary"]>;
    configurations(kind: AlertKind): Promise<AlertConfiguration[]>;
    configuration(id: string): Promise<AlertConfiguration>;
    preview(input: AlertMutation): Promise<PendingActionPublic>;
    previewCommand(input: AlertCommand): Promise<PendingActionPublic>;
    trial(input: AlertPolicySpec): Promise<AlertSchemas["AlertEvaluation"]>;
    instances(
      filter?: AlertFilter,
      history?: boolean,
    ): Promise<AlertSchemas["AlertInstancePage"]>;
    instance(id: string): Promise<AlertSchemas["AlertInstanceDetail"]>;
    handle(
      id: string,
      input: AlertSchemas["AlertHandleInput"],
    ): Promise<AlertInstance>;
    deliveries(
      filter?: AlertFilter,
    ): Promise<AlertSchemas["AlertDeliveryPage"]>;
    delivery(id: string): Promise<AlertSchemas["AlertDeliveryDetail"]>;
    exportHistory(filter?: AlertFilter): Promise<AlertSchemas["AlertExport"]>;
  };
}
