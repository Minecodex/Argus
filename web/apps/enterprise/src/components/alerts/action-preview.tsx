import type { PendingActionPublic, AlertSchemas } from "@argus/api-client";
import { useAlertText, AlertStatus, AlertCallout } from "./common";
import "../../styles/alerts.css";
export function AlertActionPreview({
  action,
  layout = "card",
  references,
}: {
  action: PendingActionPublic;
  layout?: "card" | "dialog" | "workspace";
  references?: {
    resources: Record<string, string>;
    channels: Record<string, string>;
  };
}) {
  const { t } = useAlertText();
  if (!action.action_type.startsWith("alert.")) return null;
  const preview = (action.preview ?? {}) as Record<string, unknown>;
  const spec = preview["spec"] as
    Partial<AlertSchemas["AlertPolicySpec"]> | undefined;
  const rules = Array.isArray(spec?.rules) ? spec.rules : [];
  const scope = Array.isArray(preview["resource_ids"])
    ? (preview["resource_ids"] as string[])
    : [];
  const names: NonNullable<typeof references> = references ?? {
    resources: {},
    channels: {},
  };
  if (layout !== "card" && spec?.mode) {
    const operation = String(preview["operation"] ?? "update");
    const fields = [
      [
        t("alerts.operation"),
        t(
          operation === "create"
            ? "alerts.create"
            : operation === "delete"
              ? "alerts.delete"
              : "alerts.edit",
        ),
      ],
      [
        t("alerts.origin"),
        t(preview["tool_call_id"] ? "alerts.model" : "alerts.manual"),
      ],
      [t("alerts.name"), String(preview["name"] ?? action.title)],
      [
        t("alerts.scope"),
        spec.resources?.mode === "dynamic"
          ? t("alerts.allHosts")
          : scope.map((id) => names.resources[id] ?? id).join(", ") ||
            t("alerts.all"),
      ],
      [t("alerts.combination"), t(`alerts.${spec.mode}`)],
      [
        t("alerts.ruleCount"),
        rules
          .map(
            (rule, index) =>
              `${String.fromCharCode(65 + index)}: ${rule.expression} ${rule.operator} ${rule.threshold}${rule.unit}`,
          )
          .join("\n") +
          `\n${t("alerts.combination")}: ${t(`alerts.${spec.mode}`)}`,
      ],
      [
        t("alerts.destinations"),
        (spec.channel_ids ?? [])
          .map((id) => names.channels[id] ?? id)
          .join(" · ") || "—",
      ],
      [
        t("alerts.afterChange"),
        t(spec.enabled ? "alerts.enabled" : "alerts.disabled"),
      ],
    ];
    return (
      <>
        <AlertCallout>{t("alerts.pendingHint")}</AlertCallout>
        <div
          className={
            layout === "workspace"
              ? "argus-alert-preview-fields argus-alert-preview-fields--scroll"
              : `argus-alert-preview-fields${layout === "dialog" ? " argus-alert-preview-fields--dialog" : ""}`
          }
        >
          {fields.map(([label, value]) => (
            <div className="argus-alert-preview-row" key={label}>
              <span>{label}</span>
              <span className="argus-alert-preview-value">{value}</span>
            </div>
          ))}
        </div>
      </>
    );
  }
  return (
    <div className="argus-alert-preview-scroll">
      <div className="argus-alert-preview-row">
        <span>{t("alerts.name")}</span>
        <b>{String(preview["name"] ?? action.title)}</b>
      </div>
      <div className="argus-alert-preview-row">
        <span>{t("alerts.scope")}</span>
        <span>
          {spec?.resources?.mode === "dynamic"
            ? t("alerts.allHosts")
            : scope.map((id) => names.resources[id] ?? id).join(", ") ||
              t("alerts.all")}
        </span>
      </div>
      {spec?.mode && (
        <div className="argus-alert-preview-row">
          <span>{t("alerts.combination")}</span>
          <span>{t(`alerts.${spec.mode}`)}</span>
        </div>
      )}
      {rules.map((rule, i) => (
        <section className="argus-alert-rule" key={rule.id ?? i}>
          <div className="argus-alert-rule-head">
            <b>{t("alerts.ruleLabel", { n: i + 1 })}</b>
            <AlertStatus value={rule.severity} />
          </div>
          <code>
            {rule.expression} {rule.operator} {rule.threshold}
            {rule.unit}
          </code>
          <p className="argus-alert-secondary">
            {t("alerts.recoveryThreshold")}: {rule.recovery_threshold}
          </p>
        </section>
      ))}
      {!rules.length && spec && (
        <div className="argus-alert-code-block">
          {Object.entries(spec)
            .filter(([key]) => !["credential_id", "reference_id"].includes(key))
            .map(([key, value]) => (
              <div key={key}>
                <b>
                  {key === "default_for_new_policies"
                    ? t("alerts.defaultForNewPolicies")
                    : t(`alerts.${key}`)}
                </b>
                :{" "}
                {key === "default_for_new_policies"
                  ? t(value ? "alerts.enabled" : "alerts.disabled")
                  : typeof value === "object"
                    ? JSON.stringify(value)
                    : String(value)}
              </div>
            ))}
        </div>
      )}
    </div>
  );
}
