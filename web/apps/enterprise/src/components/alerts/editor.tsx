import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  useApi,
  formatApiError,
  type AlertConfiguration,
  type AlertKind,
  type AlertMutation,
  type AlertSchemas,
  type AlertPolicySpec,
} from "@argus/api-client";
import {
  Button,
  Dialog,
  ConfirmDialog,
  Field,
  Input,
  Textarea,
  Select,
  Checkbox,
  Skeleton,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { useAlertText, AlertCallout } from "./common";
import { MatcherEditor } from "./matcher-editor";
import { PolicyForm, emptyRule } from "./policy-form";
import "../../styles/policy-form.css";
import { policyWithRuleNames } from "./rule-query";
import { NoiseRouteFields } from "./noise-route-fields";
import { defaultPolicyChannelIds } from "./policy-defaults";
type Spec = AlertConfiguration["spec"];
function initial(kind: AlertKind, configurations: AlertConfiguration[]): Spec {
  switch (kind) {
    case "policy":
      return {
        description: "",
        enabled: true,
        resources: {
          mode: "dynamic",
          types: ["host", "kubernetes_cluster"],
          ids: [],
          matchers: [],
          source_ids: [],
        },
        rules: [emptyRule()],
        mode: "independent",
        match_by: ["argus_resource_id", "argus_source_id"],
        interval_seconds: 30,
        for_seconds: 300,
        recovery_seconds: 180,
        no_data_behavior: "report",
        error_behavior: "keep",
        channel_ids: defaultPolicyChannelIds(configurations),
        route_ids: [],
        labels: {},
      };
    case "silence":
      return {
        enabled: true,
        resource_ids: [],
        policy_ids: [],
        metric: "",
        matchers: [],
        starts_at: new Date().toISOString(),
        ends_at: new Date(Date.now() + 3600000).toISOString(),
        reason: "",
      };
    case "inhibition":
      return {
        enabled: true,
        resource_ids: [],
        source: [{ key: "severity", operator: "=", value: "critical" }],
        target: [{ key: "severity", operator: "=", value: "warning" }],
        equal: ["resource_id", "policy_id"],
        reason: "",
      };
    case "channel":
      return {
        enabled: true,
        provider: "feishu",
        credential_id: "",
        display_address: "",
        mention_ids: [],
        mention_all: false,
        template: "",
      };
    case "route":
      return {
        enabled: true,
        matchers: [{ key: "severity", operator: "=", value: "critical" }],
        channel_ids: [],
        continue: false,
        priority: 0,
      };
    case "grouping":
      return {
        group_by: ["service"],
        wait_seconds: 30,
        interval_seconds: 300,
        repeat_seconds: 3600,
        timing: ["first", "repeat", "severity", "recovery"],
      };
    case "template":
      return {
        content:
          "[{{severity}}] {{policy}}\n{{resource}} · {{metric}} = {{value}}\n{{threshold}} · {{duration}}\n{{detail_url}}",
      };
    case "settings":
      return { evaluator_id: "" };
  }
}
export function AlertEditor({
  open = true,
  kind,
  row,
  duplicate,
  configurations,
  channelsReady = true,
  channelsError,
  onRetryChannels,
  names,
  onClose,
  onPreview,
}: {
  open?: boolean;
  kind: AlertKind;
  row?: AlertConfiguration;
  duplicate?: boolean;
  configurations: AlertConfiguration[];
  channelsReady?: boolean;
  channelsError?: unknown;
  onRetryChannels?: () => void;
  names: Map<string, string>;
  onClose: () => void;
  onPreview: (m: AlertMutation) => Promise<void>;
}) {
  const api = useApi(),
    { t } = useAlertText();
  const [name, setName] = useState(
    (row?.name ?? "") + (duplicate ? ` ${t("alerts.copy")}` : ""),
  );
  const [spec, setSpec] = useState<Spec>(() =>
    structuredClone(row?.spec ?? initial(kind, configurations)),
  );
  const [defaultsInitialized, setDefaultsInitialized] = useState(
    kind !== "policy" || !!row || channelsReady,
  );
  useEffect(() => {
    if (!defaultsInitialized && channelsReady) {
      setSpec(initial(kind, configurations));
      setDefaultsInitialized(true);
    }
  }, [defaultsInitialized, channelsReady, kind, configurations]);
  const [dirty, setDirty] = useState(false),
    [discard, setDiscard] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const canManageSecret = usePermission("secret.manage");
  const canManageCredential = usePermission("credential.manage");
  const canSecret = canManageSecret && canManageCredential;
  const credentials = useQuery({
    queryKey: ["secrets", "notification-credentials"],
    queryFn: () => api.secrets.listCredentials(),
    enabled: kind === "channel",
  });
  const [privateInput, setPrivateInput] = useState({
    webhook_url: "",
    signing_secret: "",
    smtp_host: "",
    smtp_port: 465,
    security: "tls",
    username: "",
    password: "",
    from: "",
    to: "",
  });
  const [createCredential, setCreateCredential] = useState(false);
  const change = <T extends Spec>(patch: Partial<T>) => {
    setSpec((old) => ({ ...old, ...patch }));
    setDirty(true);
  };
  const field = (
    key: string,
    control: ReactNode,
    hint?: string,
    required = false,
    wide = false,
  ) => (
    <Field
      density="compact"
      className={wide ? "argus-alert-field--wide" : undefined}
      label={t(`alerts.${key}`)}
      requirement={required ? "required" : "optional"}
      hint={hint}
    >
      {control}
    </Field>
  );
  const input = (
    key: string,
    value: string | number,
    onChange: (value: string) => void,
    type = "text",
    min?: number,
    max?: number,
  ) => (
    <Input
      size="lg"
      aria-label={t(`alerts.${key}`)}
      value={value}
      type={type}
      min={min}
      max={max}
      step={type === "number" ? "any" : undefined}
      onChange={(e) => onChange(e.target.value)}
    />
  );
  const select = (
    key: string,
    value: string,
    values: string[],
    onChange: (value: string) => void,
  ) => (
    <Select
      density="compact"
      size="lg"
      ariaLabel={t(`alerts.${key}`)}
      value={value}
      options={values.map((value) => ({
        value,
        label: [">", ">=", "<", "<=", "==", "!="].includes(value)
          ? value
          : t(`alerts.${value}`),
      }))}
      onValueChange={onChange}
    />
  );
  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      if (!name.trim() && !["grouping", "template"].includes(kind))
        throw new Error(t("alerts.invalid"));
      const frozen =
        kind === "policy"
          ? policyWithRuleNames(spec as AlertPolicySpec)
          : structuredClone(spec);
      if (kind === "channel" && createCredential) {
        const c = frozen as AlertSchemas["AlertChannelSpec"];
        const value =
          c.provider === "smtp"
            ? {
                ...privateInput,
                to: privateInput.to
                  .split(",")
                  .map((v) => v.trim())
                  .filter(Boolean),
              }
            : {
                webhook_url: privateInput.webhook_url,
                signing_secret: privateInput.signing_secret,
              };
        const secret = await api.secrets.create({
          name: `${name} · notification`,
          type: "notification",
          value: JSON.stringify(value),
        });
        const credential = await api.secrets.createCredential({
          name,
          protocol: "notification",
          secret_id: secret.id,
        });
        c.credential_id = credential.id;
        setPrivateInput({
          ...privateInput,
          password: "",
          signing_secret: "",
          webhook_url: "",
        });
        setCreateCredential(false);
        setSpec(frozen);
      }
      await onPreview({
        kind,
        operation: row?.id && !duplicate ? "update" : "create",
        id: row?.id && !duplicate ? row.id : undefined,
        name: name.trim() || t(`alerts.${kind}`),
        spec: frozen,
        expected_version: row?.id && !duplicate ? row.version : 0,
      });
    } catch (e) {
      setError(formatApiError(e, t("alerts.invalid"), (id) => id));
    } finally {
      setBusy(false);
    }
  };
  const title = t(
    kind === "channel" && row && !duplicate
      ? "alerts.configure"
      : row && !duplicate
        ? "alerts.edit"
        : kind === "policy"
          ? row && !duplicate
            ? "alerts.edit"
            : "alerts.create"
          : kind === "silence"
            ? "alerts.createSilence"
            : kind === "inhibition"
              ? "alerts.createInhibition"
              : kind === "channel"
                ? "alerts.createChannel"
                : kind === "route"
                  ? "alerts.createRoute"
                  : kind === "grouping"
                    ? "alerts.grouping"
                    : "alerts.template",
  );
  if (!defaultsInitialized) {
    return (
      <Dialog
        density="compact"
        open={open}
        title={title}
        onOpenChange={(v) => {
          if (!v) onClose();
        }}
      >
        {channelsError ? (
          <>
            <AlertCallout danger>
              {formatApiError(channelsError, t("alerts.invalid"), (id) => id)}
            </AlertCallout>
            <Button density="compact" onPress={onRetryChannels}>
              {t("common.retry")}
            </Button>
          </>
        ) : (
          <Skeleton height={120} />
        )}
      </Dialog>
    );
  }
  return (
    <>
      <Dialog
        density="compact"
        open={open}
        title={title}
        size="lg"
        className="argus-alert-form"
        dismissable={!busy}
        onOpenChange={(v) => {
          if (!v) {
            if (dirty) setDiscard(true);
            else onClose();
          }
        }}
        footer={
          <>
            <Button
              density="compact"
              isDisabled={busy}
              onPress={() => (dirty ? setDiscard(true) : onClose())}
            >
              {t("alerts.cancel")}
            </Button>
            <Button
              density="compact"
              variant="primary"
              isPending={busy}
              onPress={() => void submit()}
            >
              {t("alerts.preview")}
            </Button>
          </>
        }
      >
        {error && <AlertCallout danger>{error}</AlertCallout>}
        {kind === "policy" && (
          <PolicyForm
            name={name}
            policy={spec as AlertPolicySpec}
            configurations={configurations}
            names={names}
            busy={busy}
            onName={(value) => {
              setName(value);
              setDirty(true);
            }}
            onChange={(patch) => change<AlertPolicySpec>(patch)}
            onBusy={setBusy}
            onError={setError}
          />
        )}
        {kind === "silence" &&
          (() => {
            const value = spec as AlertSchemas["AlertSilenceSpec"];
            return (
              <NoiseRouteFields
                kind="silence"
                name={name}
                onName={(v) => {
                  setName(v);
                  setDirty(true);
                }}
                spec={value}
                onChange={(patch) => change(patch)}
                names={names}
                configurations={configurations}
                advanced={
                  <MatcherEditor
                    label={t("alerts.match")}
                    value={value.matchers ?? []}
                    onChange={(matchers) =>
                      change<AlertSchemas["AlertSilenceSpec"]>({ matchers })
                    }
                  />
                }
              />
            );
          })()}
        {kind === "inhibition" &&
          (() => {
            const value = spec as AlertSchemas["AlertInhibitionSpec"];
            return (
              <NoiseRouteFields
                kind="inhibition"
                name={name}
                onName={(v) => {
                  setName(v);
                  setDirty(true);
                }}
                spec={value}
                onChange={(patch) => change(patch)}
                names={names}
                configurations={configurations}
                advanced={
                  <>
                    <MatcherEditor
                      label={t("alerts.source")}
                      value={value.source}
                      onChange={(source) =>
                        change<AlertSchemas["AlertInhibitionSpec"]>({ source })
                      }
                    />
                    <MatcherEditor
                      label={t("alerts.target")}
                      value={value.target}
                      onChange={(target) =>
                        change<AlertSchemas["AlertInhibitionSpec"]>({ target })
                      }
                    />
                  </>
                }
              />
            );
          })()}
        {kind === "channel" &&
          (() => {
            const s = spec as AlertSchemas["AlertChannelSpec"];
            return (
              <>
                <div className="argus-alert-grid">
                  {field(
                    "name",
                    input("name", name, (v) => {
                      setName(v);
                      setDirty(true);
                    }),
                    undefined,
                    true,
                  )}
                  {field(
                    "provider",
                    select(
                      "provider",
                      s.provider,
                      ["dingtalk", "wecom", "feishu", "smtp"],
                      (v) =>
                        change<AlertSchemas["AlertChannelSpec"]>({
                          provider: v as typeof s.provider,
                        }),
                    ),
                  )}
                  {field(
                    "endpoint",
                    input("endpoint", s.display_address ?? "", (v) =>
                      change<AlertSchemas["AlertChannelSpec"]>({
                        display_address: v,
                      }),
                    ),
                    undefined,
                    false,
                    true,
                  )}
                  {field(
                    "credential",
                    <Select
                      density="compact"
                      size="lg"
                      ariaLabel={t("alerts.credential")}
                      value={s.credential_id}
                      options={(credentials.data ?? [])
                        .filter((c) => c.protocol === "notification")
                        .map((c) => ({ value: c.id, label: c.name }))}
                      onValueChange={(credential_id) =>
                        change<AlertSchemas["AlertChannelSpec"]>({
                          credential_id,
                        })
                      }
                    />,
                  )}
                  <Checkbox
                    appearance="outline"
                    density="compact"
                    isSelected={s.enabled}
                    onChange={(enabled) =>
                      change<AlertSchemas["AlertChannelSpec"]>({ enabled })
                    }
                  >
                    {t("alerts.enabled")}
                  </Checkbox>
                </div>
                <details className="argus-alert-drawer-section">
                  <summary>{t("alerts.notificationOptions")}</summary>
                  <Checkbox
                    appearance="outline"
                    density="compact"
                    isSelected={s.default_for_new_policies === true}
                    onChange={(default_for_new_policies) =>
                      change<AlertSchemas["AlertChannelSpec"]>({
                        default_for_new_policies,
                      })
                    }
                  >
                    {t("alerts.defaultForNewPolicies")}
                  </Checkbox>
                  <Checkbox
                    appearance="outline"
                    density="compact"
                    isDisabled={!canSecret}
                    isSelected={createCredential}
                    onChange={(v) => {
                      setCreateCredential(v);
                      setDirty(true);
                    }}
                  >
                    {t("alerts.createCredential")}
                  </Checkbox>
                  {createCredential && (
                    <div className="argus-alert-grid argus-alert-form-divider">
                      {s.provider === "smtp" ? (
                        Object.entries(privateInput)
                          .filter(
                            ([k]) =>
                              !["webhook_url", "signing_secret"].includes(k),
                          )
                          .map(([key, value]) =>
                            field(
                              key,
                              input(
                                key,
                                value,
                                (v) => {
                                  setPrivateInput((old) => ({
                                    ...old,
                                    [key]: key === "smtp_port" ? Number(v) : v,
                                  }));
                                  setDirty(true);
                                },
                                key === "password"
                                  ? "password"
                                  : key === "smtp_port"
                                    ? "number"
                                    : "text",
                              ),
                            ),
                          )
                      ) : (
                        <>
                          {field(
                            "webhookURL",
                            input(
                              "webhookURL",
                              privateInput.webhook_url,
                              (v) =>
                                setPrivateInput({
                                  ...privateInput,
                                  webhook_url: v,
                                }),
                              "url",
                            ),
                          )}
                          {field(
                            "signingSecret",
                            input(
                              "signingSecret",
                              privateInput.signing_secret,
                              (v) =>
                                setPrivateInput({
                                  ...privateInput,
                                  signing_secret: v,
                                }),
                              "password",
                            ),
                          )}
                        </>
                      )}
                    </div>
                  )}
                  {field(
                    "mention",
                    input("mention", (s.mention_ids ?? []).join(", "), (v) =>
                      change<AlertSchemas["AlertChannelSpec"]>({
                        mention_ids: v
                          .split(",")
                          .map((k) => k.trim())
                          .filter(Boolean),
                      }),
                    ),
                  )}
                  <Checkbox
                    appearance="outline"
                    density="compact"
                    isSelected={s.mention_all ?? false}
                    onChange={(mention_all) =>
                      change<AlertSchemas["AlertChannelSpec"]>({ mention_all })
                    }
                  >
                    {t("alerts.mentionAll")}
                  </Checkbox>
                </details>
                <p className="argus-alert-secondary">
                  {t("alerts.credentialsHint")}
                </p>
              </>
            );
          })()}
        {kind === "route" &&
          (() => {
            const value = spec as AlertSchemas["AlertRouteSpec"];
            return (
              <NoiseRouteFields
                kind="route"
                name={name}
                onName={(v) => {
                  setName(v);
                  setDirty(true);
                }}
                spec={value}
                onChange={(patch) => change(patch)}
                names={names}
                configurations={configurations}
                advanced={
                  <MatcherEditor
                    label={t("alerts.match")}
                    value={value.matchers}
                    onChange={(matchers) =>
                      change<AlertSchemas["AlertRouteSpec"]>({ matchers })
                    }
                  />
                }
              />
            );
          })()}
        {kind === "grouping" &&
          (() => {
            const s = spec as AlertSchemas["AlertGroupingSpec"];
            return (
              <>
                <div className="argus-alert-grid">
                  {field(
                    "groupBy",
                    input("groupBy", s.group_by.join(", "), (v) =>
                      change<AlertSchemas["AlertGroupingSpec"]>({
                        group_by: v
                          .split(",")
                          .map((k) => k.trim())
                          .filter(Boolean),
                      }),
                    ),
                  )}
                  {[
                    ["groupWait", "wait_seconds"],
                    ["groupInterval", "interval_seconds"],
                    ["repeatInterval", "repeat_seconds"],
                  ].map(([key, prop]) =>
                    field(
                      key!,
                      input(
                        key!,
                        s[prop as "wait_seconds"],
                        (v) =>
                          change<AlertSchemas["AlertGroupingSpec"]>({
                            [prop!]: Number(v),
                          }),
                        "number",
                        0,
                      ),
                    ),
                  )}
                </div>
                <div className="argus-alert-checks">
                  {["first", "repeat", "recovery", "severity"].map((v) => (
                    <Checkbox
                      key={v}
                      isSelected={s.timing.includes(
                        v as (typeof s.timing)[number],
                      )}
                      onChange={(checked) =>
                        change<AlertSchemas["AlertGroupingSpec"]>({
                          timing: checked
                            ? [...s.timing, v as (typeof s.timing)[number]]
                            : s.timing.filter((x) => x !== v),
                        })
                      }
                    >
                      {t(
                        v === "first"
                          ? "alerts.notifyFirst"
                          : v === "repeat"
                            ? "alerts.notifyRepeat"
                            : v === "recovery"
                              ? "alerts.notifyRecovery"
                              : "alerts.notifySeverity",
                      )}
                    </Checkbox>
                  ))}
                </div>
              </>
            );
          })()}
        {kind === "template" &&
          field(
            "template",
            <Textarea
              rows={8}
              value={(spec as AlertSchemas["AlertTemplateSpec"]).content}
              onChange={(e) =>
                change<AlertSchemas["AlertTemplateSpec"]>({
                  content: e.target.value,
                })
              }
            />,
            t("alerts.templateVariables"),
          )}
      </Dialog>
      <ConfirmDialog
        open={discard}
        title={t("alerts.unsaved")}
        description={t("alerts.unsaved")}
        confirmLabel={t("alerts.discard")}
        cancelLabel={t("alerts.keepEditing")}
        onOpenChange={setDiscard}
        onConfirm={onClose}
      />
    </>
  );
}
