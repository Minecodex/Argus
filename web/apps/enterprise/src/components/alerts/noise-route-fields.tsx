import type { AlertConfiguration, AlertSchemas } from "@argus/api-client";
import { Checkbox, Field, Input, Select } from "@argus/ui";
import { AlertCallout, useAlertText } from "./common";
import type { ReactNode } from "react";

type Silence = AlertSchemas["AlertSilenceSpec"];
type Inhibition = AlertSchemas["AlertInhibitionSpec"];
type Route = AlertSchemas["AlertRouteSpec"];
type Spec = Silence | Inhibition | Route;
function localDate(value: string) {
  const date = new Date(value);
  return Number.isFinite(date.getTime())
    ? new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
        .toISOString()
        .slice(0, 16)
    : "";
}
function severityOf(value: Inhibition["source"]) {
  return value.length === 1 &&
    value[0]?.key === "severity" &&
    value[0]?.operator === "="
    ? value[0].value
    : value.length === 0
      ? "all"
      : "advanced";
}

export function NoiseRouteFields({
  kind,
  name,
  onName,
  spec,
  onChange,
  names,
  configurations,
  advanced,
}: {
  kind: "silence" | "inhibition" | "route";
  name: string;
  onName: (value: string) => void;
  spec: Spec;
  onChange: (patch: Partial<Spec>) => void;
  names: Map<string, string>;
  configurations: AlertConfiguration[];
  advanced: ReactNode;
}) {
  const { t } = useAlertText();
  const field = (
    key: string,
    child: ReactNode,
    required = false,
    wide = false,
  ) => (
    <Field
      density="compact"
      className={wide ? "argus-alert-field--wide" : undefined}
      label={t(`alerts.${key}`)}
      requirement={required ? "required" : "optional"}
    >
      {child}
    </Field>
  );
  const input = (
    key: string,
    value: string | number,
    change: (value: string) => void,
    type = "text",
  ) => (
    <Input
      size="lg"
      aria-label={t(`alerts.${key}`)}
      type={type}
      value={value}
      onChange={(event) => change(event.target.value)}
    />
  );
  const select = (
    key: string,
    value: string,
    options: { value: string; label: string }[],
    change: (value: string) => void,
  ) => (
    <Select
      density="compact"
      size="lg"
      ariaLabel={t(`alerts.${key}`)}
      value={value}
      options={options}
      onValueChange={change}
    />
  );
  const title = field(
    "name",
    input("name", name, onName),
    true,
    kind === "inhibition",
  );
  const resource = (ids: string[]) =>
    field(
      "scope",
      select(
        "scope",
        ids[0] ?? "",
        [
          { value: "", label: t("alerts.allHosts") },
          ...Array.from(names, ([value, label]) => ({ value, label })),
        ],
        (value) => onChange({ resource_ids: value ? [value] : [] }),
      ),
    );
  const severity = (
    key: string,
    value: string,
    change: (value: string) => void,
  ) =>
    field(
      key,
      select(
        key,
        value,
        [
          "all",
          "critical",
          "warning",
          "info",
          ...(value === "advanced" ? ["advanced"] : []),
        ].map((value) => ({
          value,
          label: t(value === "advanced" ? "alerts.match" : `alerts.${value}`),
        })),
        change,
      ),
    );
  const details = (
    <details className="argus-alert-drawer-section">
      <summary>{t("alerts.advancedMatches")}</summary>
      {advanced}
    </details>
  );

  if (kind === "silence") {
    const value = spec as Silence;
    const policies = configurations.filter((item) => item.kind === "policy");
    const metrics = [
      ...new Set(
        policies.flatMap((item) =>
          (item.spec as AlertSchemas["AlertPolicySpec"]).rules.map(
            (rule) => rule.expression,
          ),
        ),
      ),
    ];
    if (value.metric && !metrics.includes(value.metric))
      metrics.push(value.metric);
    return (
      <>
        <AlertCallout>{t("alerts.noiseHint")}</AlertCallout>
        <div className="argus-alert-grid">
          {title}
          {resource(value.resource_ids ?? [])}
          {field(
            "policy",
            select(
              "policy",
              value.policy_ids?.[0] ?? "",
              [
                { value: "", label: t("alerts.allPolicies") },
                ...policies.map((item) => ({
                  value: item.id,
                  label: item.name,
                })),
              ],
              (id) => onChange({ policy_ids: id ? [id] : [] }),
            ),
          )}
          {field(
            "metric",
            select(
              "metric",
              value.metric ?? "",
              [
                { value: "", label: t("alerts.allMetrics") },
                ...metrics.map((value) => ({ value, label: value })),
              ],
              (metric) => onChange({ metric }),
            ),
          )}
          {field(
            "durationHours",
            input(
              "durationHours",
              (new Date(value.ends_at).getTime() -
                new Date(value.starts_at).getTime()) /
                3_600_000,
              (hours) =>
                onChange({
                  ends_at: new Date(
                    new Date(value.starts_at).getTime() +
                      Number(hours) * 3_600_000,
                  ).toISOString(),
                }),
              "number",
            ),
          )}
          {field(
            "until",
            input(
              "until",
              localDate(value.ends_at),
              (end) => {
                if (end) onChange({ ends_at: new Date(end).toISOString() });
              },
              "datetime-local",
            ),
          )}
          {field(
            "reason",
            input("reason", value.reason, (reason) => onChange({ reason })),
            true,
          )}
        </div>
        {details}
      </>
    );
  }
  if (kind === "inhibition") {
    const value = spec as Inhibition;
    return (
      <>
        <AlertCallout>{t("alerts.noiseHint")}</AlertCallout>
        <div className="argus-alert-grid">
          {title}
          {severity("sourceSeverity", severityOf(value.source), (severity) =>
            onChange({
              source:
                severity === "all"
                  ? []
                  : [{ key: "severity", operator: "=", value: severity }],
            }),
          )}
          {severity("targetSeverity", severityOf(value.target), (severity) =>
            onChange({
              target:
                severity === "all"
                  ? []
                  : [{ key: "severity", operator: "=", value: severity }],
            }),
          )}
          {field(
            "equal",
            input("equal", value.equal.join(", "), (equal) =>
              onChange({
                equal: equal
                  .split(",")
                  .map((item) => item.trim())
                  .filter(Boolean),
              }),
            ),
          )}
          {field(
            "reason",
            input("reason", value.reason, (reason) => onChange({ reason })),
            true,
          )}
        </div>
        {details}
      </>
    );
  }
  const value = spec as Route;
  return (
    <>
      <div className="argus-alert-grid">
        {title}
        {severity("severity", severityOf(value.matchers), (severity) =>
          onChange({
            matchers:
              severity === "all"
                ? []
                : [{ key: "severity", operator: "=", value: severity }],
          }),
        )}
      </div>
      <h3>{t("alerts.destinations")}</h3>
      <div className="argus-alert-checks argus-alert-drawer-section">
        {configurations
          .filter((item) => item.kind === "channel")
          .map((channel) => (
            <Checkbox
              key={channel.id}
              appearance="outline"
              density="compact"
              isSelected={value.channel_ids.includes(channel.id)}
              onChange={(selected) =>
                onChange({
                  channel_ids: selected
                    ? [...value.channel_ids, channel.id]
                    : value.channel_ids.filter((id) => id !== channel.id),
                })
              }
            >
              {channel.name}
            </Checkbox>
          ))}
      </div>
      <details className="argus-alert-drawer-section">
        <summary>{t("alerts.advancedMatches")}</summary>
        {advanced}
        {field(
          "priority",
          input(
            "priority",
            value.priority,
            (priority) => onChange({ priority: Number(priority) }),
            "number",
          ),
        )}
        <Checkbox
          isSelected={value.continue}
          onChange={(next) => onChange({ continue: next })}
        >
          {t("alerts.continueRoutes")}
        </Checkbox>
      </details>
    </>
  );
}
