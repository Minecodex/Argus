import { useState, useMemo } from "react";
import { useQuery, useQueries, useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  Plus,
  Search,
  Download,
  List,
  Check,
  SlidersHorizontal,
  Info,
} from "lucide-react";
import {
  useApi,
  formatApiError,
  type AlertKind,
  type AlertConfiguration,
  type AlertMutation,
  type AlertInstance,
  type AlertDelivery,
  type PendingActionPublic,
  type AlertFilter,
} from "@argus/api-client";
import {
  ActionMenu,
  Button,
  Input,
  Select,
  Checkbox,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Dialog,
  Skeleton,
  Field,
  type Column,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { PendingActionConfirm } from "../components/hosts/pending-action-confirm";
import { AlertEditor } from "../components/alerts/editor";
import { AlertDetails } from "../components/alerts/details";
import { AlertSettings } from "../components/alerts/settings";
import {
  AlertStatus,
  AlertTable,
  AlertCallout,
  SmallAction,
  useAlertText,
  policySpec,
  resourceName,
  clock,
  duration,
} from "../components/alerts/common";
import "../styles/alerts.css";
import { useAlertNavigation } from "../components/alerts/navigation";
const TABS = [
  "active",
  "history",
  "deliveries",
  "policies",
  "noise",
  "channels",
] as const;
const KINDS: AlertKind[] = [
  "policy",
  "silence",
  "inhibition",
  "channel",
  "route",
  "grouping",
  "template",
];
export function AlertsPage() {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useAlertText();
  const canRead = usePermission("alert.read"),
    canPolicy = usePermission("alert.policy.manage"),
    canNoise = usePermission("alert.silence.manage"),
    canNotify = usePermission("alert.notification.manage"),
    canHandle = usePermission("alert.handle");
  const {
    tab,
    changeTab,
    filter,
    setFilter,
    cursors,
    setCursors,
    detail,
    setDetail,
  } = useAlertNavigation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [grouped, setGrouped] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [editor, setEditor] = useState<{
    kind: AlertKind;
    row?: AlertConfiguration;
    duplicate?: boolean;
  } | null>(null);
  const summary = useQuery({
    queryKey: ["alerts", "summary"],
    queryFn: () => api.alerts.summary(),
    enabled: canRead,
    refetchInterval: 30000,
  });
  const configs = useQueries({
    queries: KINDS.map((kind) => ({
      queryKey: ["alerts", "configurations", kind],
      queryFn: () => api.alerts.configurations(kind),
      enabled: canRead,
    })),
  });
  const configurations = configs.flatMap((c) => c.data ?? []);
  const policies = configurations.filter((c) => c.kind === "policy"),
    channels = configurations.filter((c) => c.kind === "channel");
  const events = useQuery({
    queryKey: ["alerts", tab, filter],
    queryFn: () => api.alerts.instances(filter, tab === "history"),
    enabled: canRead && ["active", "history"].includes(tab),
    refetchInterval: 30000,
  });
  const deliveries = useQuery({
    queryKey: ["alerts", "deliveries", filter],
    queryFn: () => api.alerts.deliveries(filter),
    enabled: canRead && tab === "deliveries",
    refetchInterval: 30000,
  });
  const hosts = useQuery({
    queryKey: ["hosts", "alert-labels"],
    queryFn: () => api.hosts.list(),
    enabled: canRead,
  });
  const clusters = useQuery({
    queryKey: ["clusters", "alert-labels"],
    queryFn: () => api.kubernetes.listClusters(),
    enabled: canRead,
  });
  const names = useMemo(
    () =>
      new Map(
        [...(hosts.data?.items ?? []), ...(clusters.data?.items ?? [])].map(
          (h) => [h.id, h.name],
        ),
      ),
    [hosts.data, clusters.data],
  );
  const report = (e: unknown) =>
    setError(formatApiError(e, t("alerts.invalid"), (id) => id));
  const refresh = () => {
    void cache.invalidateQueries({ queryKey: ["alerts"] });
    setSelected(new Set());
  };
  const editFilter = (patch: AlertFilter) => {
    setFilter((old) => ({ ...old, ...patch, cursor: undefined }));
    setCursors([]);
  };
  const prepare = async (m: AlertMutation) => {
    setError("");
    try {
      setPending(await api.alerts.preview(m));
    } catch (e) {
      report(e);
      throw e;
    }
  };
  const command = async (
    operation: "notification.test" | "notification.resend",
    id: string,
    version: number,
  ) => {
    try {
      setPending(
        await api.alerts.previewCommand({
          operation,
          id,
          expected_version: version,
        }),
      );
    } catch (e) {
      report(e);
    }
  };
  const handle = async (e: AlertInstance) => {
    try {
      await api.alerts.handle(e.id, {
        operation: "acknowledge",
        expected_handling_version: e.handling_version,
      });
      refresh();
    } catch (err) {
      report(err);
    }
  };
  const exportHistory = async () => {
    try {
      const value = await api.alerts.exportHistory(filter);
      const url = URL.createObjectURL(
        new Blob([value.content], { type: value.content_type }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = value.filename;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      report(e);
    }
  };
  const eventColumns: Column<AlertInstance>[] = [
    {
      key: "select",
      selection: true,
      header: (
        <Checkbox
          appearance="outline"
          aria-label={t("alerts.bulkAck")}
          isSelected={
            !!events.data?.items.length &&
            events.data.items.every((e) => selected.has(e.id))
          }
          onChange={(v) =>
            setSelected(
              v ? new Set(events.data?.items.map((e) => e.id)) : new Set(),
            )
          }
        />
      ),
      render: (e) => (
        <Checkbox
          appearance="outline"
          aria-label={
            e.state.delivery_gap ? t("alerts.deliveryGapTitle") : e.state.name
          }
          isSelected={selected.has(e.id)}
          onChange={(v) =>
            setSelected((old) => {
              const next = new Set(old);
              if (v) next.add(e.id);
              else next.delete(e.id);
              return next;
            })
          }
        />
      ),
    },
    {
      key: "event",
      header: t("alerts.event"),
      render: (e) => (
        <div className="argus-alert-event-cell">
          <span
            className={`argus-alert-severity-bar argus-alert-severity-bar--${e.severity}`}
          />
          <Button
            density="compact"
            className="argus-button--text"
            variant="ghost"
            onPress={() => setDetail({ type: "instance", id: e.id })}
          >
            {e.state.delivery_gap ? t("alerts.deliveryGapTitle") : e.state.name}
          </Button>
        </div>
      ),
    },
    {
      key: "resource",
      header: t("alerts.resource"),
      render: (e) => (
        <>
          <div>{resourceName(e, names)}</div>
          <div className="argus-alert-secondary">
            <AlertStatus value={e.severity} />
          </div>
        </>
      ),
    },
    {
      key: "value",
      header: t("alerts.currentValue"),
      render: (e) => (
        <>
          {e.state.value === undefined ? (
            <AlertStatus
              value={e.state.delivery_gap ? "delivery_fault" : e.health}
              tone={e.state.delivery_gap ? "danger" : undefined}
            />
          ) : (
            <>
              <div
                className={`argus-alert-mono ${e.lifecycle === "resolved" ? "argus-alert-success-text" : "argus-alert-observed"}`}
              >
                {e.state.value}
                {e.state.unit}
              </div>
              <div className="argus-alert-secondary">
                {tab === "history" ? (
                  t(`alerts.${e.lifecycle}`)
                ) : (
                  <>
                    {e.state.policy_snapshot.rules.find(
                      (r) => r.id === e.state.rule_id,
                    )?.operator ?? ">"}{" "}
                    {e.state.threshold}
                    {e.state.unit}
                  </>
                )}
              </div>
            </>
          )}
        </>
      ),
    },
    {
      key: "notification",
      header: t("alerts.notification"),
      render: (e) => (
        <>
          <AlertStatus value={e.state.notification_state} />
          <div className="argus-alert-secondary">
            {e.acknowledged
              ? t("alerts.acknowledged")
              : t("alerts.unacknowledged")}
          </div>
        </>
      ),
    },
    {
      key: "duration",
      header: t("alerts.duration"),
      render: (e) => (
        <>
          <span>{duration(e, t, summary.data?.last_evaluated_at)}</span>
          <div className="argus-alert-secondary">
            {clock(tab === "history" ? e.resolved_at : e.started_at)}
          </div>
        </>
      ),
    },
    {
      key: "owner",
      header: t("alerts.owner"),
      render: (e) => (
        <span>
          {e.state.labels["owner_name"] ??
            e.owner_id ??
            t("alerts.notAssigned")}
        </span>
      ),
    },
    {
      key: "action",
      header: t("alerts.action"),
      render: (e) => (
        <ActionMenu
          items={[
            {
              label: t("alerts.details"),
              onSelect: () => setDetail({ type: "instance", id: e.id }),
            },
            ...(tab !== "history"
              ? [
                  {
                    label: t("alerts.ackShort"),
                    onSelect: () => void handle(e),
                    disabled: !canHandle || e.acknowledged,
                  },
                ]
              : []),
          ]}
        />
      ),
    },
  ];
  const deliveryColumns: Column<AlertDelivery>[] = [
    {
      key: "event",
      header: t("alerts.event"),
      render: (d) => (
        <>
          <Button
            density="compact"
            variant="ghost"
            className="argus-button--text"
            onPress={() => setDetail({ type: "delivery", id: d.id })}
          >
            {d.message.subject}
          </Button>
          <div className="argus-alert-secondary">
            {d.resource_ids.map((id) => names.get(id) ?? id).join(", ")}
          </div>
        </>
      ),
    },
    {
      key: "channel",
      header: t("alerts.channel"),
      render: (d) => {
        const channel = channels.find((c) => c.id === d.channel_id);
        return (
          <>
            {channel?.name ?? d.channel_id}
            <div className="argus-alert-secondary">
              {channel
                ? t(`alerts.${(channel.spec as { provider: string }).provider}`)
                : "—"}
            </div>
          </>
        );
      },
    },
    {
      key: "result",
      header: t("alerts.result"),
      render: (d) => (
        <>
          <AlertStatus value={d.status} />
          <div className="argus-alert-secondary">
            {d.last_error
              ? t(`alerts.${d.last_error}`)
              : t("alerts.acceptedReason")}
          </div>
        </>
      ),
    },
    {
      key: "attempt",
      header: t("alerts.attempts"),
      cellClassName: "argus-table__cell--small argus-table__cell--mono",
      render: (d) => d.attempt,
    },
    {
      key: "time",
      header: t("alerts.sentAt"),
      cellClassName: "argus-table__cell--small argus-table__cell--mono",
      render: (d) => (
        <span className="argus-alert-mono">
          {clock(d.last_attempt_at, true)}
        </span>
      ),
    },
    {
      key: "action",
      header: t("alerts.action"),
      render: (d) => (
        <ActionMenu
          items={[
            {
              label: t("alerts.details"),
              onSelect: () => setDetail({ type: "delivery", id: d.id }),
            },
            {
              label: t("alerts.resend"),
              onSelect: () =>
                void command("notification.resend", d.id, d.version),
              disabled: !canNotify || !["failed", "unknown"].includes(d.status),
            },
          ]}
        />
      ),
    },
  ];
  const policyColumns: Column<AlertConfiguration>[] = [
    {
      key: "name",
      header: t("alerts.policy"),
      render: (p) => (
        <>
          <Button
            density="compact"
            className="argus-button--text"
            variant="ghost"
            onPress={() => setDetail({ type: "policy", id: p.id })}
          >
            {p.managed_by === "collector_delivery"
              ? t("alerts.deliveryGapTitle")
              : p.name}
          </Button>
          <div className="argus-alert-secondary">
            {t(
              p.managed_by === "collector_delivery"
                ? "alerts.deliveryManagedPolicyLabel"
                : policySpec(p).enabled
                  ? "alerts.enabled"
                  : "alerts.disabled",
            )}{" "}
            · v{p.version}
          </div>
        </>
      ),
    },
    {
      key: "rules",
      header: t("alerts.ruleCount"),
      render: (p) => (
        <>
          <span className="argus-alert-count">
            {policySpec(p).rules.length}
          </span>{" "}
          {t("alerts.ruleCount")}
          <div className="argus-alert-secondary">
            {t(`alerts.${policySpec(p).mode}`)}
          </div>
        </>
      ),
    },
    {
      key: "scope",
      header: t("alerts.scope"),
      render: (p) =>
        policySpec(p).resources.mode === "dynamic"
          ? t("alerts.allHosts")
          : (policySpec(p).resources.ids ?? [])
              .map((id) => names.get(id) ?? id)
              .join(", "),
    },
    {
      key: "health",
      header: t("alerts.health"),
      render: (p) => (
        <>
          <AlertStatus value={p.health} />
          <div className="argus-alert-secondary">
            {clock(p.last_evaluated_at, true)}
          </div>
        </>
      ),
    },
    {
      key: "action",
      header: t("alerts.action"),
      render: (p) => (
        <ActionMenu
          items={[
            {
              label: t("alerts.edit"),
              onSelect: () => setEditor({ kind: "policy", row: p }),
              disabled: !canPolicy || p.managed_by === "collector_delivery",
            },
            {
              label: t(
                policySpec(p).enabled ? "alerts.pause" : "alerts.resume",
              ),
              onSelect: () =>
                void prepare({
                  kind: "policy",
                  operation: policySpec(p).enabled ? "disable" : "enable",
                  id: p.id,
                  expected_version: p.version,
                }),
              disabled: !canPolicy || p.managed_by === "collector_delivery",
            },
          ]}
        />
      ),
    },
  ];
  if (!canRead)
    return (
      <div className="argus-alert-page">
        <h1>{t("alerts.center")}</h1>
        <AlertCallout danger>{t("alerts.forbidden")}</AlertCallout>
      </div>
    );
  const page = tab === "deliveries" ? deliveries.data : events.data;
  const pageFooter = page ? (
    <div className="argus-alert-table-footer">
      <span>
        {t("alerts.pageInfo", {
          total: page.total,
          page: cursors.length + 1,
          pages: Math.max(1, Math.ceil(page.total / (filter.limit ?? 5))),
        })}
      </span>
      <div className="argus-alert-actions">
        <SmallAction
          disabled={!cursors.length}
          onPress={() => {
            const prev = [...cursors];
            const cursor = prev.pop();
            setCursors(prev);
            setFilter({
              ...filter,
              cursor: cursor || undefined,
            });
          }}
        >
          {t("alerts.previous")}
        </SmallAction>
        <SmallAction
          disabled={!page.next_cursor}
          onPress={() => {
            setCursors([...cursors, filter.cursor ?? ""]);
            setFilter({
              ...filter,
              cursor: page.next_cursor ?? undefined,
            });
          }}
        >
          {t("alerts.next")}
        </SmallAction>
      </div>
    </div>
  ) : null;
  const filteredPolicies = policies.filter(
    (p) =>
      !filter.query ||
      JSON.stringify(p).toLowerCase().includes(filter.query.toLowerCase()),
  );
  const loading =
    summary.isLoading ||
    (["active", "history"].includes(tab) && events.isLoading) ||
    (tab === "deliveries" && deliveries.isLoading);
  const failed =
    error ||
    (events.isError && ["active", "history"].includes(tab)
      ? formatApiError(events.error, t("alerts.invalid"), (id) => id)
      : deliveries.isError && tab === "deliveries"
        ? formatApiError(deliveries.error, t("alerts.invalid"), (id) => id)
        : summary.isError
          ? formatApiError(summary.error, t("alerts.invalid"), (id) => id)
          : "");
  return (
    <div className="argus-alert-page" data-testid="alert-center">
      <header className="argus-alert-hero">
        <div>
          <div className="argus-alert-eyebrow">{t("alerts.eyebrow")}</div>
          <h1>{t("alerts.center")}</h1>
          <p className="argus-alert-description">{t("alerts.hero")}</p>
        </div>
        <div className="argus-alert-actions">
          <Button
            density="compact"
            size="lg"
            variant="primary"
            isDisabled={!canPolicy}
            onPress={() => setEditor({ kind: "policy" })}
          >
            <Plus size={16} />
            {t("alerts.create")}
          </Button>
        </div>
      </header>
      <section className="argus-alert-stats" aria-label={t("alerts.center")}>
        {(
          [
            [
              "activeCount",
              "includesQuiet",
              summary.data?.active ?? 0,
              "danger",
              [10, 15, 12, 26, 20, 30, 18, 24, 26, 20],
            ],
            [
              "unhandled",
              "handlingHint",
              summary.data?.unacknowledged ?? 0,
              "warning",
              [24, 27, 20, 18, 20, 10, 12, 10, 16, 14],
            ],
            [
              "policyCount",
              "multiHint",
              summary.data?.policies ?? 0,
              "info",
              [8, 8, 8, 13, 13, 13, 23, 23, 23, 23],
            ],
            [
              "failedCount",
              "failedHint",
              summary.data?.failed_deliveries ?? 0,
              "warning",
              [3, 3, 3, 16, 3, 3, 3, 3, 14, 8],
            ],
          ] as [string, string, number, string, number[]][]
        ).map(([label, hint, value, tone, spark]) => (
          <div key={label} className="argus-alert-stat">
            <div>
              <div className="argus-alert-stat-label">
                {label === "policyCount" ? (
                  <SlidersHorizontal size={16} />
                ) : (
                  <Bell size={16} />
                )}
                {t(`alerts.${String(label)}`)}
              </div>
              <div className="argus-alert-stat-value">{value}</div>
              <div className="argus-alert-stat-caption">
                {t(`alerts.${String(hint)}`)}
              </div>
            </div>
            <svg
              className={`argus-alert-spark is-${tone}`}
              viewBox="0 0 80 38"
              aria-hidden
            >
              <polyline
                points={(spark as number[])
                  .map((point, index) => `${index * 8},${36 - point}`)
                  .join(" ")}
                fill="none"
                stroke="currentColor"
                strokeWidth="1.7"
              />
            </svg>
          </div>
        ))}
      </section>
      {failed && (
        <AlertCallout danger>
          {failed}
          <Button
            density="compact"
            variant="ghost"
            onPress={() => {
              setError("");
              refresh();
            }}
          >
            {t("alerts.refresh")}
          </Button>
        </AlertCallout>
      )}
      <Tabs
        density="compact"
        value={tab}
        onValueChange={(value) => {
          changeTab(value);
          setSelected(new Set());
        }}
      >
        <TabsList>
          {TABS.map((key) => (
            <TabsTrigger key={key} value={key}>
              {t(`alerts.${key}`)}
              {key === "active" && (
                <span className="argus-alert-count" aria-hidden="true">
                  {summary.data?.current_total ?? 0}
                </span>
              )}
              {key === "policies" && (
                <span className="argus-alert-count" aria-hidden="true">
                  {summary.data?.policies ?? 0}
                </span>
              )}
            </TabsTrigger>
          ))}
        </TabsList>
        {TABS.map((key) => (
          <TabsContent key={key} value={key}>
            {tab === key && (
              <>
                {["active", "history", "deliveries", "policies"].includes(
                  tab,
                ) && (
                  <div className="argus-alert-toolbar">
                    <div className="argus-alert-filters">
                      <div className="argus-alert-search">
                        <Search size={16} className="argus-alert-icon" />
                        <Input
                          size="lg"
                          density="compact"
                          aria-label={t("alerts.search")}
                          placeholder={t("alerts.search")}
                          value={filter.query ?? ""}
                          onChange={(e) =>
                            editFilter({ query: e.target.value })
                          }
                        />
                      </div>
                      {["active", "history"].includes(tab) && (
                        <Select
                          density="compact"
                          size="lg"
                          ariaLabel={t("alerts.allLevels")}
                          sizing="content"
                          value={filter.severity ?? ""}
                          options={[
                            { value: "", label: t("alerts.allLevels") },
                            ...["critical", "warning", "info"].map((value) => ({
                              value,
                              label: t(`alerts.${value}`),
                            })),
                          ]}
                          onValueChange={(severity) => editFilter({ severity })}
                        />
                      )}
                      {tab === "active" && (
                        <Select
                          density="compact"
                          size="lg"
                          ariaLabel={t("alerts.allHandling")}
                          sizing="content"
                          value={
                            filter.acknowledged === undefined
                              ? ""
                              : String(filter.acknowledged)
                          }
                          options={[
                            { value: "", label: t("alerts.allHandling") },
                            {
                              value: "false",
                              label: t("alerts.unacknowledged"),
                            },
                            { value: "true", label: t("alerts.acknowledged") },
                          ]}
                          onValueChange={(v) =>
                            editFilter({
                              acknowledged: v === "" ? undefined : v === "true",
                            })
                          }
                        />
                      )}
                      {tab === "history" && (
                        <Select
                          density="compact"
                          size="lg"
                          ariaLabel={t("alerts.allLifecycle")}
                          sizing="content"
                          value={filter.lifecycle ?? ""}
                          options={[
                            { value: "", label: t("alerts.allLifecycle") },
                            ...[
                              "firing",
                              "pending",
                              "recovering",
                              "resolved",
                              "closed",
                            ].map((value) => ({
                              value,
                              label: t(`alerts.${value}`),
                            })),
                          ]}
                          onValueChange={(lifecycle) =>
                            editFilter({ lifecycle })
                          }
                        />
                      )}
                      {tab === "deliveries" && (
                        <Select
                          density="compact"
                          size="lg"
                          ariaLabel={t("alerts.notification")}
                          sizing="content"
                          value={filter.notification_state ?? ""}
                          options={[
                            { value: "", label: t("alerts.allStates") },
                            ...[
                              "pending",
                              "sending",
                              "accepted",
                              "failed",
                              "unknown",
                              "muted",
                              "inhibited",
                              "canceled",
                            ].map((value) => ({
                              value,
                              label: t(`alerts.${value}`),
                            })),
                          ]}
                          onValueChange={(notification_state) =>
                            editFilter({ notification_state })
                          }
                        />
                      )}
                    </div>
                    <div className="argus-alert-actions">
                      {tab === "active" && (
                        <>
                          <Button
                            density="compact"
                            size="lg"
                            onPress={() => setGrouped(!grouped)}
                          >
                            <List size={16} />
                            {t(grouped ? "alerts.flat" : "alerts.group")}
                          </Button>
                          <Button
                            density="compact"
                            size="lg"
                            isDisabled={!canHandle || !selected.size}
                            onPress={() =>
                              void (async () => {
                                for (const e of events.data?.items ?? []) {
                                  if (selected.has(e.id) && !e.acknowledged)
                                    await handle(e);
                                }
                              })()
                            }
                          >
                            <Check size={16} />
                            {t("alerts.bulkAck")}
                          </Button>
                        </>
                      )}
                      {tab === "history" && (
                        <Button
                          density="compact"
                          size="lg"
                          onPress={() => void exportHistory()}
                        >
                          <Download size={16} />
                          {t("alerts.export")}
                        </Button>
                      )}
                      {tab === "policies" && (
                        <Button
                          density="compact"
                          size="lg"
                          variant="primary"
                          isDisabled={!canPolicy}
                          onPress={() => setEditor({ kind: "policy" })}
                        >
                          <Plus size={16} />
                          {t("alerts.create")}
                        </Button>
                      )}
                    </div>
                  </div>
                )}
                {tab === "history" && (
                  <div className="argus-alert-history-dates">
                    <Field
                      label={t("alerts.fromDate")}
                      requirement="optional"
                      density="compact"
                      className="argus-alert-field"
                    >
                      <Input
                        size="lg"
                        density="compact"
                        type="date"
                        aria-label={t("alerts.fromDate")}
                        value={filter.from?.slice(0, 10) ?? ""}
                        onChange={(e) =>
                          editFilter({
                            from: e.target.value
                              ? `${e.target.value}T00:00:00+08:00`
                              : undefined,
                          })
                        }
                      />
                    </Field>
                    <Field
                      label={t("alerts.toDate")}
                      requirement="optional"
                      density="compact"
                      className="argus-alert-field"
                    >
                      <Input
                        size="lg"
                        density="compact"
                        type="date"
                        aria-label={t("alerts.toDate")}
                        value={filter.to?.slice(0, 10) ?? ""}
                        onChange={(e) =>
                          editFilter({
                            to: e.target.value
                              ? `${e.target.value}T23:59:59+08:00`
                              : undefined,
                          })
                        }
                      />
                    </Field>
                  </div>
                )}
                {loading ? (
                  <Skeleton height={200} />
                ) : (
                  <>
                    {["active", "history"].includes(tab) &&
                      (grouped ? (
                        Object.entries(
                          (events.data?.items ?? []).reduce<
                            Record<string, AlertInstance[]>
                          >((groups, e) => {
                            (groups[resourceName(e, names)] ??= []).push(e);
                            return groups;
                          }, {}),
                        ).map(([name, items]) => (
                          <section key={name}>
                            <div className="argus-alert-group-heading">
                              {name}{" "}
                              <span className="argus-alert-count">
                                {items?.length ?? 0}
                              </span>
                            </div>
                            <AlertTable
                              data={items ?? []}
                              footer={false}
                              columns={eventColumns}
                            />
                          </section>
                        ))
                      ) : (
                        <AlertTable
                          data={events.data?.items ?? []}
                          footer={pageFooter}
                          columns={
                            tab === "history"
                              ? eventColumns.slice(1)
                              : eventColumns
                          }
                        />
                      ))}
                    {tab === "deliveries" && (
                      <AlertTable
                        data={deliveries.data?.items ?? []}
                        footer={pageFooter}
                        columns={deliveryColumns}
                      />
                    )}{" "}
                    {tab === "policies" && (
                      <>
                        <AlertTable
                          data={filteredPolicies}
                          columns={policyColumns}
                        />
                        <p className="argus-alert-footer-note">
                          <Info size={16} aria-hidden />
                          {t("alerts.healthNote")}
                        </p>
                      </>
                    )}
                    {["noise", "channels"].includes(tab) && (
                      <AlertSettings
                        tab={tab}
                        configurations={configurations}
                        names={names}
                        canNoise={canNoise}
                        canNotify={canNotify}
                        onEdit={(kind, row) => setEditor({ kind, row })}
                        onPrepare={prepare}
                        onTest={command}
                        query={filter.query}
                        onQuery={(query) => editFilter({ query })}
                      />
                    )}
                  </>
                )}
              </>
            )}
          </TabsContent>
        ))}
      </Tabs>
      {editor && (
        <AlertEditor
          open={!pending}
          key={`${editor.kind}:${editor.row?.id ?? "new"}:${editor.duplicate ?? false}`}
          {...editor}
          configurations={configurations}
          channelsReady={configs[3]!.isSuccess && !configs[3]!.isFetching}
          channelsError={configs[3]!.error}
          onRetryChannels={() => {
            void configs[3]!.refetch();
          }}
          names={names}
          onClose={() => setEditor(null)}
          onPreview={prepare}
        />
      )}
      {pending && (
        <Dialog
          density="compact"
          open
          title={t("alerts.preview")}
          size="lg"
          className="argus-alert-form"
          onOpenChange={(v) => {
            if (!v) setPending(null);
          }}
        >
          <PendingActionConfirm
            layout="dialog"
            alertReferences={{
              resources: Object.fromEntries(names),
              channels: Object.fromEntries(
                channels.map((channel) => [channel.id, channel.name]),
              ),
            }}
            action={pending}
            confirmLabel={t("alerts.confirm")}
            onDone={() => {
              setPending(null);
              setEditor(null);
              refresh();
            }}
            onCancel={() => setPending(null)}
          />
        </Dialog>
      )}
      {detail && (
        <AlertDetails
          {...detail}
          names={names}
          configurations={configurations}
          canHandle={canHandle}
          canNoise={canNoise}
          canPolicy={canPolicy}
          canNotify={canNotify}
          onResend={(id, version) => {
            setDetail(null);
            void command("notification.resend", id, version);
          }}
          onClose={() => setDetail(null)}
          onEdit={(p) => {
            setDetail(null);
            setEditor({ kind: "policy", row: p });
          }}
          onDuplicate={(p) => {
            setDetail(null);
            setEditor({ kind: "policy", row: p, duplicate: true });
          }}
          onSilence={(e) => {
            setDetail(null);
            setEditor({
              kind: "silence",
              row: {
                id: "",
                kind: "silence",
                name: e.state.name,
                spec: {
                  enabled: true,
                  resource_ids: e.resource_ids,
                  policy_ids: [e.policy_id],
                  metric: e.state.metric,
                  matchers: [],
                  starts_at: new Date().toISOString(),
                  ends_at: new Date(Date.now() + 3600000).toISOString(),
                  reason: "",
                },
                version: 0,
                generation: 1,
                health: "normal",
                created_at: "",
                updated_at: "",
              },
            });
          }}
          onRefresh={refresh}
        />
      )}
    </div>
  );
}
