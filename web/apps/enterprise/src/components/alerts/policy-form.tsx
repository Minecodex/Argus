import { useRef, useState, type ReactNode } from "react";
import { Plus, Search, Trash2 } from "lucide-react";
import {
  useApi,
  formatApiError,
  type AlertConfiguration,
  type AlertPolicySpec,
  type AlertRule,
  type AlertSchemas,
} from "@argus/api-client";
import {
  Button,
  Card,
  CardHeader,
  CardContent,
  Checkbox,
  Field,
  Input,
  JsonField,
  SegmentedControl,
  Select,
  Switch,
  Textarea,
  ValueSelector,
} from "@argus/ui";
import { useAlertText, AlertCallout, AlertStatus } from "./common";
import {
  canUseRuleBuilder,
  ruleEditingMode,
  policyWithRuleNames,
} from "./rule-query";
import { MatcherEditor } from "./matcher-editor";

export const emptyRule = (): AlertRule => ({
  id: crypto.randomUUID(),
  name: "",
  expression: "host_cpu_usage_percent",
  query_mode: "numeric",
  operator: ">",
  threshold: 85,
  recovery_threshold: 75,
  severity: "critical",
  unit: "%",
});

export function PolicyForm({
  name,
  policy: p,
  configurations,
  names,
  busy,
  onName,
  onChange,
  onBusy,
  onError,
}: {
  name: string;
  policy: AlertPolicySpec;
  configurations: AlertConfiguration[];
  names: Map<string, string>;
  busy: boolean;
  onName: (name: string) => void;
  onChange: (patch: Partial<AlertPolicySpec>) => void;
  onBusy: (busy: boolean) => void;
  onError: (error: string) => void;
}) {
  const { t } = useAlertText(),
    api = useApi();
  const [modes, setModes] = useState<Record<string, string>>({});
  const [trial, setTrial] = useState<{
    identity: string;
    result: AlertSchemas["AlertEvaluation"];
  }>();
  const identity = JSON.stringify(p),
    current = useRef(identity);
  current.current = identity;
  const updateRule = (index: number, patch: Partial<AlertRule>) =>
    onChange({
      rules: p.rules.map((rule, i) =>
        i === index ? { ...rule, ...patch } : rule,
      ),
    });
  const field = (
    key: string,
    control: ReactNode,
    required = false,
    wide = false,
    hint?: string,
  ) => (
    <Field
      density="compact"
      label={t(`alerts.${key}`)}
      requirement={required ? "required" : "optional"}
      className={wide ? "argus-field--wide" : undefined}
      hint={hint}
    >
      {control}
    </Field>
  );
  const number = (
    key: string,
    value: number,
    update: (value: number) => void,
    min?: number,
    max?: number,
  ) => (
    <Input
      aria-label={t(`alerts.${key}`)}
      type="number"
      step="any"
      min={min}
      max={max}
      value={value}
      onChange={(e) => update(Number(e.target.value))}
    />
  );
  const choice = (
    key: string,
    value: string,
    values: string[],
    update: (value: string) => void,
  ) => (
    <Select
      ariaLabel={t(`alerts.${key}`)}
      value={value}
      density="compact"
      options={values.map((v) => ({
        value: v,
        label: /^[<>=!]+$/.test(v) ? v : t(`alerts.${v}`),
      }))}
      onValueChange={update}
    />
  );
  const channels = configurations.filter((c) => c.kind === "channel");
  const routes = configurations.filter((c) => c.kind === "route");
  const test = async () => {
    onBusy(true);
    onError("");
    const captured = identity;
    try {
      const result = await api.alerts.trial(policyWithRuleNames(p));
      if (current.current === captured)
        setTrial({ identity: captured, result });
    } catch (e) {
      onError(formatApiError(e, t("alerts.invalid"), (id) => id));
    } finally {
      onBusy(false);
    }
  };
  return (
    <div className="argus-policy-form">
      <Card>
        <CardHeader
          title={t("alerts.policyBasics")}
          description={t("alerts.policyBasicsHint")}
          action={
            <Switch
              label={t("alerts.activate")}
              checked={p.enabled}
              onChange={(enabled) => onChange({ enabled })}
            />
          }
        />
        <CardContent>
          <div className="argus-config-grid argus-policy-basics">
            {field(
              "name",
              <Input value={name} onChange={(e) => onName(e.target.value)} />,
              true,
            )}
            <Field
              density="compact"
              label={t("alerts.scope")}
              requirement="required"
              controlMode="group"
            >
              <ValueSelector
                label={t("alerts.scope")}
                multiple
                presentation="popover"
                value={{
                  all: p.resources.mode === "dynamic",
                  values: p.resources.ids ?? [],
                }}
                allLabel={t("alerts.authorizedResources")}
                candidates={[...names.keys()]}
                valueLabels={Object.fromEntries(names)}
                onChange={(selection) =>
                  onChange({
                    resources: {
                      ...p.resources,
                      mode: selection.all ? "dynamic" : "explicit",
                      ids: selection.all ? [] : selection.values,
                    },
                  })
                }
              />
            </Field>
            {field(
              "combination",
              choice(
                "combination",
                p.mode,
                ["independent", "any", "every"],
                (mode) => onChange({ mode: mode as AlertPolicySpec["mode"] }),
              ),
              false,
              false,
              t("alerts.modeHelp"),
            )}
            {field(
              "description",
              <Textarea
                rows={2}
                value={p.description ?? ""}
                onChange={(e) => onChange({ description: e.target.value })}
              />,
              false,
              true,
            )}
          </div>
          <details className="argus-policy-advanced">
            <summary>{t("alerts.scopeAdvanced")}</summary>
            <MatcherEditor
              label={t("alerts.match")}
              value={p.resources.matchers ?? []}
              onChange={(matchers) =>
                onChange({ resources: { ...p.resources, matchers } })
              }
            />
            <div className="argus-config-grid">
              {field(
                "matchBy",
                <Input
                  value={(p.match_by ?? []).join(", ")}
                  onChange={(e) =>
                    onChange({
                      match_by: e.target.value
                        .split(",")
                        .map((v) => v.trim())
                        .filter(Boolean),
                    })
                  }
                />,
              )}
              {field(
                "sourceIds",
                <Input
                  value={(p.resources.source_ids ?? []).join(", ")}
                  onChange={(e) =>
                    onChange({
                      resources: {
                        ...p.resources,
                        source_ids: e.target.value
                          .split(",")
                          .map((v) => v.trim())
                          .filter(Boolean),
                      },
                    })
                  }
                />,
              )}
            </div>
          </details>
        </CardContent>
      </Card>
      <Card>
        <CardHeader
          title={t("alerts.policyConditions")}
          description={t("alerts.policyConditionsHint")}
          action={
            <Button
              density="compact"
              size="sm"
              isDisabled={p.rules.length >= 32}
              onPress={() => onChange({ rules: [...p.rules, emptyRule()] })}
            >
              <Plus aria-hidden />
              {t("alerts.addRule")}
            </Button>
          }
        />
        <CardContent>
          <div className="argus-alert-rules">
            {p.rules.map((rule, i) => {
              const key = rule.id ?? String(i),
                mode = modes[key] ?? ruleEditingMode(rule);
              return (
                <section
                  className="argus-policy-rule"
                  key={key}
                  aria-label={t("alerts.ruleLabel", { n: i + 1 })}
                >
                  <div className="argus-policy-rule__header">
                    <strong>{t("alerts.ruleLabel", { n: i + 1 })}</strong>
                    <SegmentedControl
                      label={t("alerts.queryMode")}
                      density="compact"
                      value={mode}
                      options={[
                        { value: "builder", label: t("alerts.simpleBuilder") },
                        { value: "advanced", label: t("alerts.advancedQuery") },
                      ]}
                      onChange={(next) => {
                        if (next === "builder" && !canUseRuleBuilder(rule)) {
                          onError(t("alerts.builderConversionError"));
                          return;
                        }
                        setModes((old) => ({ ...old, [key]: next }));
                        onError("");
                      }}
                    />
                    <Button
                      density="compact"
                      size="sm"
                      variant="ghost"
                      isDisabled={p.rules.length <= 1}
                      aria-label={`${t("alerts.remove")} ${t("alerts.ruleLabel", { n: i + 1 })}`}
                      onPress={() =>
                        onChange({ rules: p.rules.filter((_, n) => n !== i) })
                      }
                    >
                      <Trash2 aria-hidden />
                      {t("alerts.remove")}
                    </Button>
                  </div>
                  {mode === "builder"
                    ? field(
                        "metric",
                        <Input
                          value={rule.expression}
                          onChange={(e) =>
                            updateRule(i, {
                              expression: e.target.value,
                              name: e.target.value,
                            })
                          }
                        />,
                        true,
                      )
                    : field(
                        "promqlExpression",
                        <Textarea
                          rows={3}
                          value={rule.expression}
                          spellCheck={false}
                          onChange={(e) =>
                            updateRule(i, { expression: e.target.value })
                          }
                        />,
                        true,
                      )}
                  <div className="argus-policy-rule__conditions">
                    {field(
                      "operator",
                      choice(
                        "operator",
                        rule.operator,
                        [">", ">=", "<", "<=", "==", "!="],
                        (operator) =>
                          updateRule(i, {
                            operator: operator as AlertRule["operator"],
                          }),
                      ),
                    )}
                    {field(
                      "threshold",
                      number("threshold", rule.threshold, (threshold) =>
                        updateRule(i, { threshold }),
                      ),
                    )}
                    {field(
                      "recoveryThreshold",
                      number(
                        "recoveryThreshold",
                        rule.recovery_threshold,
                        (recovery_threshold) =>
                          updateRule(i, { recovery_threshold }),
                      ),
                    )}
                    {field(
                      "unit",
                      <Input
                        value={rule.unit ?? ""}
                        onChange={(e) =>
                          updateRule(i, { unit: e.target.value })
                        }
                      />,
                    )}
                    {field(
                      "severity",
                      choice(
                        "severity",
                        rule.severity,
                        ["critical", "warning", "info"],
                        (severity) =>
                          updateRule(i, {
                            severity: severity as AlertRule["severity"],
                          }),
                      ),
                    )}
                  </div>
                  <p className="argus-policy-rule__summary">
                    {t("alerts.conditionSummary", {
                      expression: rule.expression,
                      operator: rule.operator,
                      threshold: rule.threshold,
                      unit: rule.unit ?? "",
                    })}
                  </p>
                  {mode === "advanced" && (
                    <details className="argus-policy-advanced">
                      <summary>{t("alerts.queryMode")}</summary>
                      {field(
                        "queryMode",
                        choice(
                          "queryMode",
                          rule.query_mode,
                          ["numeric", "predicate"],
                          (query_mode) =>
                            updateRule(i, {
                              query_mode: query_mode as AlertRule["query_mode"],
                            }),
                        ),
                      )}
                      {rule.query_mode === "predicate" &&
                        field(
                          "presence",
                          <Input
                            value={rule.presence_expression ?? ""}
                            onChange={(e) =>
                              updateRule(i, {
                                presence_expression: e.target.value,
                              })
                            }
                          />,
                          p.no_data_behavior === "report",
                          false,
                          t("alerts.missingPredicateHelp"),
                        )}
                    </details>
                  )}
                </section>
              );
            })}
          </div>
          <div className="argus-policy-trial">
            <Button
              density="compact"
              isPending={busy}
              onPress={() => void test()}
            >
              <Search aria-hidden />
              {t("alerts.previewQuery")}
            </Button>
            <span className="argus-alert-secondary">
              {t("alerts.trialHint")}
            </span>
          </div>
          {trial?.identity === identity && (
            <AlertCallout>
              <AlertStatus value={trial.result.health} />
              <span>
                {trial.result.observations
                  .map(
                    (o) =>
                      `${o.rule.name} = ${o.value} (${t(o.firing ? "alerts.firing" : "alerts.normal")})`,
                  )
                  .join(" · ") || trial.result.error_code}
              </span>
            </AlertCallout>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader
          title={t("alerts.policyEvaluation")}
          description={t("alerts.policyEvaluationHint")}
        />
        <CardContent>
          <div className="argus-config-grid">
            {field(
              "evaluationInterval",
              number(
                "evaluationInterval",
                p.interval_seconds,
                (interval_seconds) => onChange({ interval_seconds }),
                10,
                3600,
              ),
              true,
            )}
            {field(
              "forDuration",
              number(
                "forDuration",
                p.for_seconds / 60,
                (minutes) => onChange({ for_seconds: minutes * 60 }),
                0,
                1440,
              ),
              true,
            )}
            {field(
              "recoveryDuration",
              number(
                "recoveryDuration",
                p.recovery_seconds / 60,
                (minutes) => onChange({ recovery_seconds: minutes * 60 }),
                0,
                1440,
              ),
              true,
            )}
          </div>
          <details className="argus-policy-advanced">
            <summary>{t("alerts.exceptionHandling")}</summary>
            <div className="argus-config-grid">
              {field(
                "noDataBehavior",
                choice(
                  "noDataBehavior",
                  p.no_data_behavior,
                  ["report", "keep"],
                  (v) => onChange({ no_data_behavior: v as "report" | "keep" }),
                ),
              )}
              {field(
                "errorBehavior",
                choice(
                  "errorBehavior",
                  p.error_behavior,
                  ["report", "keep"],
                  (v) => onChange({ error_behavior: v as "report" | "keep" }),
                ),
              )}
            </div>
          </details>
        </CardContent>
      </Card>
      <Card>
        <CardHeader
          title={t("alerts.policyNotifications")}
          description={t("alerts.policyNotificationsHint")}
        />
        <CardContent>
          <div className="argus-policy-channels">
            {channels.map((channel) => (
              <div className="argus-policy-channel" key={channel.id}>
                <Checkbox
                  aria-label={channel.name}
                  appearance="outline"
                  density="compact"
                  isSelected={(p.channel_ids ?? []).includes(channel.id)}
                  onChange={(selected) =>
                    onChange({
                      channel_ids: selected
                        ? [...(p.channel_ids ?? []), channel.id]
                        : p.channel_ids?.filter((id) => id !== channel.id),
                    })
                  }
                >
                  <span>
                    {channel.name}
                    <small>
                      {t(
                        `alerts.${(channel.spec as AlertSchemas["AlertChannelSpec"]).provider}`,
                      )}
                    </small>
                  </span>
                </Checkbox>
              </div>
            ))}
            {!channels.length && (
              <p className="argus-alert-secondary">
                {t("alerts.emptyChannel")}
              </p>
            )}
          </div>
          <details className="argus-policy-advanced">
            <summary>{t("alerts.routingAndLabels")}</summary>
            <div className="argus-alert-checks">
              {routes.map((route) => (
                <Checkbox
                  key={route.id}
                  appearance="outline"
                  isSelected={(p.route_ids ?? []).includes(route.id)}
                  onChange={(selected) =>
                    onChange({
                      route_ids: selected
                        ? [...(p.route_ids ?? []), route.id]
                        : p.route_ids?.filter((id) => id !== route.id),
                    })
                  }
                >
                  {route.name}
                </Checkbox>
              ))}
            </div>
            <JsonField
              label={t("alerts.policyLabels")}
              kind="object"
              value={p.labels ?? {}}
              onChange={(value) => {
                if (Object.values(value).every((v) => typeof v === "string"))
                  onChange({ labels: value as Record<string, string> });
                else onError(t("alerts.labelsMustBeStrings"));
              }}
            />
          </details>
        </CardContent>
      </Card>
    </div>
  );
}
