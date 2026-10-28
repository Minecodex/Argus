import { alertMeasurement } from "./measurement";
import { useState } from "react";
import { AlertEventTimeline } from "./event-timeline";
import { CollectorDeliveryGap } from "./delivery-gap";
import { Check, BellOff } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEnterpriseAuthStore } from "@argus/auth";
import { usePermission } from "../../lib/permissions";
import {
  useApi,
  formatApiError,
  type AlertConfiguration,
  type AlertInstance,
} from "@argus/api-client";
import {
  DrawerPanel,
  Button,
  Field,
  Textarea,
  Select,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  MetricChart,
  Skeleton,
} from "@argus/ui";
import {
  AlertStatus,
  AlertCallout,
  useAlertText,
  clock,
  datetime,
  resourceName,
  duration,
  policySpec,
} from "./common";
export function AlertDetails({
  type,
  id,
  names,
  configurations,
  canHandle,
  canNoise,
  canPolicy,
  canNotify,
  onResend,
  onClose,
  onEdit,
  onDuplicate,
  onSilence,
  onRefresh,
}: {
  type: "instance" | "delivery" | "policy";
  id: string;
  names: Map<string, string>;
  configurations: AlertConfiguration[];
  canHandle: boolean;
  canNoise: boolean;
  canPolicy: boolean;
  canNotify: boolean;
  onResend: (id: string, version: number) => void;
  onClose: () => void;
  onEdit: (c: AlertConfiguration) => void;
  onDuplicate: (c: AlertConfiguration) => void;
  onSilence: (e: AlertInstance) => void;
  onRefresh: () => void;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t, locale } = useAlertText();
  const [tab, setTab] = useState("overview"),
    [comment, setComment] = useState(""),
    [owner, setOwner] = useState<string | null>(null),
    [error, setError] = useState("");
  const [handlingPending, setHandlingPending] = useState(false);
  const currentUser = useEnterpriseAuthStore((state) => state.session?.user);
  const canReadUsers = usePermission("user.read");
  const instance = useQuery({
    queryKey: ["alerts", "instance", id],
    queryFn: () => api.alerts.instance(id),
    enabled: type === "instance",
  });
  const delivery = useQuery({
    queryKey: ["alerts", "delivery", id],
    queryFn: () => api.alerts.delivery(id),
    enabled: type === "delivery",
  });
  const policy = useQuery({
    queryKey: ["alerts", "policy", id],
    queryFn: () => api.alerts.configuration(id),
    enabled: type === "policy",
  });
  const users = useQuery({
    queryKey: ["org", "alert-owners"],
    queryFn: () => api.org.listUsers(),
    enabled: type === "instance" && canHandle && canReadUsers,
  });
  const related = useQuery({
    queryKey: ["alerts", "instance-deliveries", id],
    queryFn: () => api.alerts.deliveries({ query: id, limit: 500 }),
    enabled: type === "instance" && tab === "deliveries",
  });
  const e = instance.data?.instance;
  const chart = useQuery({
    queryKey: ["alerts", "trend", id, e?.version],
    queryFn: () =>
      api.telemetry.queryMetricsRange({
        query: e!.state.metric,
        resource_ids: e!.resource_ids,
        time_range: {
          from: new Date(
            new Date(e!.last_evaluated_at).getTime() - 3600000,
          ).toISOString(),
          to: e!.last_evaluated_at,
        },
        step_seconds: 60,
        budget: {
          max_scan_bytes: 268435456,
          max_rows: 1000,
          max_samples: 10000,
          max_series: 1000,
          timeout_ms: 10000,
          max_result_bytes: 1048576,
        },
      }),
    enabled:
      type === "instance" &&
      !!e &&
      !e.state.delivery_gap &&
      !!e.state.metric &&
      tab === "overview",
  });
  const handling = async (operation: "acknowledge" | "assign" | "comment") => {
    if (operation === "comment" && !comment.trim()) {
      setError(t("alerts.commentPlaceholder"));
      return;
    }
    if (!e || handlingPending) return;
    setHandlingPending(true);
    try {
      await api.alerts.handle(e.id, {
        operation,
        expected_handling_version: e.handling_version,
        owner_id:
          operation === "assign"
            ? (owner ?? e.owner_id) || undefined
            : undefined,
        comment: operation === "comment" ? comment : undefined,
      });
      setError("");
      if (operation === "comment") setComment("");
      await cache.invalidateQueries({ queryKey: ["alerts", "instance", id] });
      onRefresh();
    } catch (err) {
      setError(formatApiError(err, t("alerts.invalid"), (id) => id));
    } finally {
      setHandlingPending(false);
    }
  };
  const timeline = instance.data?.events ?? [];
  const p = policy.data;
  const d = delivery.data?.delivery;
  const title =
    e?.state.delivery_gap || p?.managed_by === "collector_delivery"
      ? t("alerts.deliveryGapTitle")
      : (e?.state.name ?? p?.name ?? d?.message.subject ?? t("alerts.details"));
  const cells = (values: Array<[string, string]>) => (
    <div className="argus-alert-detail-grid">
      {values.map(([label, value]) => (
        <div key={label}>
          <span className="argus-alert-secondary">{t(`alerts.${label}`)}</span>
          <div className="argus-alert-detail-value">{value}</div>
        </div>
      ))}
    </div>
  );
  const rows = chart.data?.data.result as
    | Array<{
        metric?: Record<string, string>;
        values?: Array<[number, string]>;
      }>
    | undefined;
  const series = (Array.isArray(rows) ? rows : [])
    .filter((r) => Array.isArray(r.values))
    .map((r) => ({
      name: Object.entries(r.metric ?? {})
        .map(([k, v]) => `${k}=${v}`)
        .join(", "),
      points: r.values!.map((v) => Number(v[1])).filter(Number.isFinite),
    }));
  return (
    <DrawerPanel
      density="compact"
      headerLeading={
        e ? (
          <>
            <AlertStatus value={e.severity} />
            <AlertStatus value={e.lifecycle} />
          </>
        ) : undefined
      }
      open
      title={title}
      description={
        e
          ? [resourceName(e, names), e.id, e.state.labels["service"]]
              .filter(Boolean)
              .join(" · ")
          : undefined
      }
      width="calc(var(--drawer-width) + var(--space-8))"
      className="argus-alert-details"
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      {error && <AlertCallout danger>{error}</AlertCallout>}
      {(instance.isError || delivery.isError || policy.isError) && (
        <AlertCallout danger>
          {formatApiError(
            instance.error ?? delivery.error ?? policy.error,
            t("alerts.invalid"),
            (id) => id,
          )}
        </AlertCallout>
      )}
      {type === "instance" &&
        (!e ? (
          <Skeleton height={140} />
        ) : (
          <>
            <div className="argus-alert-detail-tabs">
              <Tabs density="segmented" value={tab} onValueChange={setTab}>
                <TabsList>
                  {["overview", "timeline", "deliveries"].map((key) => (
                    <TabsTrigger key={key} value={key}>
                      {t(`alerts.${key}`)}
                    </TabsTrigger>
                  ))}
                </TabsList>
                <TabsContent value="overview">
                  {e.health !== "normal" &&
                    e.health !== "not_evaluated" &&
                    !e.state.delivery_gap && (
                      <AlertCallout danger>
                        {t("alerts.queryTimeout")}
                      </AlertCallout>
                    )}
                  {cells([
                    ...(e.state.delivery_gap
                      ? []
                      : [
                          [
                            "currentValue",
                            `${alertMeasurement(e.state.value, e.state.unit, locale, e.state.threshold)} / ${alertMeasurement(e.state.threshold, e.state.unit, locale, e.state.value ?? undefined)}`,
                          ] as [string, string],
                        ]),
                    ["policy", e.state.policy_name],
                    ["duration", duration(e, t)],
                    ["triggered", datetime(e.started_at)],
                    ["recovered", datetime(e.resolved_at)],
                    ["notification", t(`alerts.${e.state.notification_state}`)],
                    ["policyRevision", `v${e.policy_version}`],
                    [
                      e.state.delivery_gap ? "deliveryHealth" : "health",
                      t(
                        e.state.delivery_gap
                          ? "alerts.delivery_fault"
                          : `alerts.${e.health}`,
                      ),
                    ],
                  ])}
                  {e.state.delivery_gap ? (
                    <CollectorDeliveryGap report={e.state.delivery_gap} />
                  ) : (
                    <>
                      <pre className="argus-alert-code-block">
                        {e.state.metric}{" "}
                        {e.state.policy_snapshot.rules.find(
                          (r) => r.id === e.state.rule_id,
                        )?.operator ?? ">"}{" "}
                        {e.state.threshold}
                      </pre>
                      <section className="argus-alert-drawer-section">
                        <h3>{t("alerts.trend")}</h3>
                        {chart.isError ? (
                          <AlertCallout danger>
                            {formatApiError(
                              chart.error,
                              t("alerts.invalid"),
                              (id) => id,
                            )}
                          </AlertCallout>
                        ) : chart.isPending ? (
                          <Skeleton height={160} />
                        ) : series.length ? (
                          <MetricChart
                            appearance="compact"
                            domain={e.state.unit === "%" ? [0, 120] : undefined}
                            threshold={{
                              value: e.state.threshold,
                              label: `${t("alerts.threshold")} ${e.state.threshold}`,
                            }}
                            height={160}
                            series={series}
                            labels={rows?.[0]?.values?.map((v) =>
                              clock(new Date(v[0] * 1000).toISOString()),
                            )}
                            ariaLabel={t("alerts.trend")}
                          />
                        ) : (
                          <p className="argus-alert-empty">
                            {t("alerts.noData")}
                          </p>
                        )}
                        <p className="argus-alert-secondary">
                          {t("alerts.versionSnapshot")}
                        </p>
                      </section>
                    </>
                  )}
                  <section className="argus-alert-drawer-section">
                    <h3>{t("alerts.timeline")}</h3>
                    <AlertEventTimeline
                      events={timeline}
                      lastEvaluatedAt={e.last_evaluated_at}
                    />
                  </section>
                  {e.state.noise_reasons.length > 0 && (
                    <AlertCallout>
                      {e.state.noise_reasons.join(" · ")}
                    </AlertCallout>
                  )}
                </TabsContent>
                <TabsContent value="timeline">
                  <AlertEventTimeline events={timeline} />
                </TabsContent>
                <TabsContent value="deliveries">
                  {related.isPending ? (
                    <Skeleton height={80} />
                  ) : related.isError ? (
                    <AlertCallout danger>
                      {formatApiError(
                        related.error,
                        t("alerts.invalid"),
                        (id) => id,
                      )}
                    </AlertCallout>
                  ) : (
                    related.data?.items.map((d) => (
                      <div key={d.id} className="argus-alert-result-row">
                        <div>
                          {configurations.find((c) => c.id === d.channel_id)
                            ?.name ?? d.channel_id}
                          <p className="argus-alert-secondary">
                            {clock(d.last_attempt_at)} · {d.attempt}{" "}
                            {t("alerts.attempts")}
                          </p>
                          <p className="argus-alert-secondary">
                            {d.last_error}
                          </p>
                        </div>
                        <AlertStatus value={d.status} />
                      </div>
                    ))
                  )}
                </TabsContent>
              </Tabs>
            </div>
            <div className="argus-alert-form-divider">
              <div className="argus-alert-actions">
                <Button
                  density="compact"
                  variant="primary"
                  isDisabled={!canHandle || handlingPending || e.acknowledged}
                  onPress={() => void handling("acknowledge")}
                >
                  <Check size={16} aria-hidden />
                  {t("alerts.acknowledge")}
                </Button>
                <Button
                  density="compact"
                  isDisabled={
                    !canNoise || ["resolved", "closed"].includes(e.lifecycle)
                  }
                  onPress={() => onSilence(e)}
                >
                  <BellOff size={16} aria-hidden />
                  {t("alerts.silence")}
                </Button>
              </div>
              <div className="argus-alert-grid argus-alert-drawer-section">
                <Field
                  label={t("alerts.owner")}
                  requirement="optional"
                  density="compact"
                >
                  <Select
                    density="compact"
                    size="lg"
                    value={owner ?? e.owner_id ?? ""}
                    ariaLabel={t("alerts.owner")}
                    options={[
                      { value: "", label: t("alerts.notAssigned") },
                      ...(users.data ?? [])
                        .filter((u) => u.status === "active")
                        .map((u) => ({ value: u.id, label: u.displayName })),
                      ...(currentUser &&
                      !users.data?.some((u) => u.id === currentUser.id)
                        ? [
                            {
                              value: currentUser.id,
                              label: currentUser.display_name,
                            },
                          ]
                        : []),
                    ]}
                    onValueChange={setOwner}
                    disabled={!canHandle || handlingPending}
                  />
                </Field>
                <Field
                  label={t("alerts.claim")}
                  requirement="none"
                  density="compact"
                >
                  <Button
                    density="compact"
                    className="argus-button--field"
                    isDisabled={!canHandle || handlingPending}
                    onPress={() => void handling("assign")}
                  >
                    {t("alerts.save")}
                  </Button>
                </Field>
              </div>
              <Field
                label={t("alerts.comment")}
                requirement="optional"
                density="compact"
              >
                <Textarea
                  value={comment}
                  placeholder={t("alerts.commentPlaceholder")}
                  disabled={!canHandle}
                  onChange={(event) => setComment(event.target.value)}
                />
              </Field>
              <div className="argus-alert-actions argus-alert-actions--end">
                <Button
                  density="compact"
                  isDisabled={!canHandle || handlingPending}
                  onPress={() => void handling("comment")}
                >
                  {t("alerts.addComment")}
                </Button>
              </div>
            </div>
          </>
        ))}
      {type === "policy" && p && (
        <>
          {p.managed_by === "collector_delivery" && (
            <AlertCallout>{t("alerts.deliveryManagedPolicy")}</AlertCallout>
          )}
          <AlertStatus value={p.health} />
          {cells([
            ["combination", t(`alerts.${policySpec(p).mode}`)],
            ["evaluationInterval", String(policySpec(p).interval_seconds)],
            ["forDuration", String(policySpec(p).for_seconds / 60)],
            ["policyRevision", `v${p.version}`],
          ])}
          {policySpec(p).rules.map((r, i) => (
            <section key={r.id ?? i} className="argus-alert-rule">
              <h3>{r.name}</h3>
              <pre className="argus-alert-code-block">
                {r.expression} {r.operator} {r.threshold}
              </pre>
              <AlertStatus value={r.severity} />
              <p className="argus-alert-secondary">
                {t("alerts.recoveryThreshold")}: {r.recovery_threshold}
              </p>
            </section>
          ))}
          <div className="argus-alert-actions">
            <Button
              density="compact"
              isDisabled={!canPolicy || p.managed_by === "collector_delivery"}
              onPress={() => onEdit(p)}
            >
              {t("alerts.edit")}
            </Button>
            <Button
              density="compact"
              isDisabled={!canPolicy || p.managed_by === "collector_delivery"}
              onPress={() => onDuplicate(p)}
            >
              {t("alerts.copy")}
            </Button>
          </div>
        </>
      )}
      {type === "delivery" && d && (
        <>
          <AlertStatus value={d.status} />
          {canNotify && ["failed", "unknown"].includes(d.status) && (
            <Button density="compact" onPress={() => onResend(d.id, d.version)}>
              {t("alerts.resend")}
            </Button>
          )}
          {cells([
            [
              "channel",
              configurations.find((c) => c.id === d.channel_id)?.name ??
                d.channel_id,
            ],
            ["attempts", String(d.attempt)],
            ["sentAt", clock(d.last_attempt_at)],
            ["result", d.last_error || t("alerts.acceptedReason")],
          ])}
          <pre className="argus-alert-code-block">{d.message.text}</pre>
          <h3>{t("alerts.attempts")}</h3>
          <ol className="argus-alert-timeline">
            {delivery.data?.attempts.map((a) => (
              <li key={a.id}>
                <AlertStatus value={a.status} />
                <p>
                  {a.error_code} · {a.result_code}
                </p>
                <p className="argus-alert-secondary">
                  {clock(a.started_at)} — {clock(a.finished_at)}
                </p>
              </li>
            ))}
          </ol>
        </>
      )}
    </DrawerPanel>
  );
}
