import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  auditPresentationKey,
  humanizeAuditCode,
  AUDIT_ACTION_CODES,
  AUDIT_RESOURCE_TYPE_CODES,
  formatApiError,
  useApi,
} from "@argus/api-client";
import type { AuditEvent, AuditResult } from "@argus/api-client";
import {
  Badge,
  Alert,
  ActionGroup,
  CodeBlock,
  Button,
  DataTable,
  EmptyState,
  FilterBar,
  FormDrawer,
  KeyValueGrid,
  PageShell,
  RowAction,
  Spinner,
  StatusBadge,
} from "@argus/ui";
import "../styles/settings.css";
import { formatDateTime } from "../components/settings/shared";
import { usePermission } from "../lib/permissions";

type AuditRow = {
  id: string;
  createdAt: string;
  actorName: string;
  actionLabel: string;
  resource: string;
  origin: AuditEvent["origin"];
  result: AuditResult;
  summary: string;
};

const TIME_RANGES = [
  { value: "", hours: 0 },
  { value: "24h", hours: 24 },
  { value: "7d", hours: 24 * 7 },
  { value: "30d", hours: 24 * 30 },
] as const;

function resultTone(result: AuditResult) {
  if (result === "success") return "success" as const;
  if (result === "denied") return "warning" as const;
  return "danger" as const;
}

/** 企业审计：FilterBar 过滤 + 行点击详情抽屉。 */
export function SettingsAuditPage() {
  const { t } = useTranslation();
  const api = useApi();

  const [actor, setActor] = useState("");
  const [actionType, setActionType] = useState("");
  const [resourceType, setResourceType] = useState("");
  const [result, setResult] = useState("");
  const [timeRange, setTimeRange] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<AuditEvent | null>(null);
  const [cursor, setCursor] = useState<string | undefined>();
  const [previous, setPrevious] = useState<Array<string | undefined>>([]);
  const [from, setFrom] = useState<string | undefined>();
  const resetPage = () => {
    setCursor(undefined);
    setPrevious([]);
  };
  const change = (setter: (value: string) => void) => (value: string) => {
    setter(value);
    resetPage();
  };
  const labelFor = useCallback(
    (kind: "actions" | "resourceTypes" | "actorTypes", code: string) =>
      t(auditPresentationKey("settings.audit", kind, code), {
        defaultValue: humanizeAuditCode(code),
      }),
    [t],
  );
  const actorLabel = (item: AuditEvent) =>
    item.actorName !== item.actorUserId
      ? item.actorUsername && item.actorUsername !== item.actorName
        ? `${item.actorName} (${item.actorUsername})`
        : item.actorName
      : labelFor("actorTypes", item.actorType);
  const resourceLabel = (item: AuditEvent) => {
    if (!item.resourceType) return "—";
    const typeLabel = labelFor("resourceTypes", item.resourceType);
    return item.resourceName
      ? `${typeLabel} · ${item.resourceName}`
      : typeLabel;
  };
  const actionLabel = (item: AuditEvent) => labelFor("actions", item.action);
  const summaryLabel = (item: AuditEvent) =>
    item.resourceName
      ? t("settings.audit.summaryWithResource", {
          action: actionLabel(item),
          resource: item.resourceName,
        })
      : actionLabel(item);

  const users = useQuery({
    queryKey: ["org", "users", "facet"],
    queryFn: () => api.org.listUsers(),
    enabled: usePermission("identity.read"),
  });

  const events = useQuery({
    queryKey: [
      "audit",
      { actor, actionType, resourceType, result, search, from, cursor },
    ],
    queryFn: () =>
      api.audit.list(
        {
          action: actionType || undefined,
          actorUserId: actor || undefined,
          resourceType: resourceType || undefined,
          result: (result || undefined) as AuditResult | undefined,
          query: search || undefined,
          from,
        },
        { page: { cursor, limit: 50 } },
      ),
  });

  // Registered codes remain selectable even when absent from the current page.
  const resourceTypeOptions = useMemo(() => {
    const set = new Set<string>(AUDIT_RESOURCE_TYPE_CODES);
    for (const item of events.data?.items ?? []) {
      if (item.resourceType) set.add(item.resourceType);
    }
    return [...set]
      .sort()
      .map((value) => ({ value, label: labelFor("resourceTypes", value) }));
  }, [events.data, labelFor]);
  const filtered = events.data?.items ?? [];
  const actionOptions = [
    ...new Set([...AUDIT_ACTION_CODES, ...filtered.map((item) => item.action)]),
  ];
  const actorOptions = new Map(
    (users.data ?? []).map((user) => [user.id, user.displayName]),
  );
  for (const event of filtered)
    actorOptions.set(event.actorUserId, actorLabel(event));
  if (actor && !actorOptions.has(actor)) actorOptions.set(actor, actor);

  const rows: AuditRow[] = filtered.map((item) => ({
    id: item.id,
    createdAt: item.createdAt,
    actorName: actorLabel(item),
    actionLabel: actionLabel(item),
    resource: resourceLabel(item),
    origin: item.origin,
    result: item.result,
    summary: summaryLabel(item),
  }));

  const openDetail = (row: AuditRow) =>
    setSelected(filtered.find((item) => item.id === row.id) ?? null);

  return (
    <PageShell
      description={t("settings.audit.description")}
      title={t("settings.audit.title")}
    >
      <div className="argus-settings-stack">
        <FilterBar
          filters={[
            {
              key: "actor",
              value: actor,
              allLabel: t("settings.audit.filters.allActors"),
              ariaLabel: t("settings.audit.filters.actor"),
              options: [...actorOptions].map(([value, label]) => ({
                value,
                label,
              })),
              onChange: change(setActor),
            },
            {
              key: "actionType",
              value: actionType,
              allLabel: t("settings.audit.filters.allActions"),
              ariaLabel: t("settings.audit.filters.actionType"),
              options: actionOptions.map((type) => ({
                value: type,
                label: labelFor("actions", type),
              })),
              onChange: change(setActionType),
            },
            {
              key: "resourceType",
              value: resourceType,
              allLabel: t("settings.audit.filters.allResources"),
              ariaLabel: t("settings.audit.filters.resourceType"),
              options: resourceTypeOptions,
              onChange: change(setResourceType),
            },
            {
              key: "result",
              value: result,
              allLabel: t("settings.audit.filters.allResults"),
              ariaLabel: t("settings.audit.filters.result"),
              options: (["success", "failure", "denied"] as const).map(
                (value) => ({
                  value,
                  label: t(`settings.audit.results.${value}`),
                }),
              ),
              onChange: change(setResult),
            },
            {
              key: "timeRange",
              value: timeRange,
              allLabel: t("settings.audit.filters.allTime"),
              ariaLabel: t("settings.audit.filters.timeRange"),
              options: [
                { value: "24h", label: t("settings.audit.filters.last24h") },
                { value: "7d", label: t("settings.audit.filters.last7d") },
                { value: "30d", label: t("settings.audit.filters.last30d") },
              ],
              onChange: (value) => {
                setTimeRange(value);
                const hours =
                  TIME_RANGES.find((r) => r.value === value)?.hours ?? 0;
                setFrom(
                  hours
                    ? new Date(Date.now() - hours * 3600000).toISOString()
                    : undefined,
                );
                resetPage();
              },
            },
          ]}
          onRefresh={() => {
            resetPage();
            void events.refetch();
          }}
          refreshing={events.isFetching}
          search={{
            value: search,
            onChange: change(setSearch),
            placeholder: t("settings.audit.filters.searchPlaceholder"),
          }}
        />

        {events.error ? (
          <Alert
            tone="danger"
            title={t("settings.audit.loadFailed")}
            description={formatApiError(
              events.error,
              t("settings.audit.loadFailed"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
          />
        ) : events.isPending ? (
          <Spinner />
        ) : rows.length === 0 ? (
          <EmptyState description="" title={t("settings.audit.empty")} />
        ) : (
          <div className="argus-audit-table">
            <DataTable<AuditRow>
              columns={[
                {
                  key: "createdAt",
                  header: t("settings.audit.table.time"),
                  render: (row) => formatDateTime(row.createdAt),
                },
                {
                  key: "actorName",
                  header: t("settings.audit.table.actor"),
                },
                {
                  key: "actionLabel",
                  header: t("settings.audit.table.action"),
                },
                { key: "resource", header: t("settings.audit.table.resource") },
                {
                  key: "origin",
                  header: t("settings.audit.table.origin"),
                  render: (row) => (
                    <Badge>{t(`settings.audit.origins.${row.origin}`)}</Badge>
                  ),
                },
                {
                  key: "result",
                  header: t("settings.audit.table.result"),
                  render: (row) => (
                    <StatusBadge tone={resultTone(row.result)}>
                      {t(`settings.audit.results.${row.result}`)}
                    </StatusBadge>
                  ),
                },
                {
                  key: "id",
                  header: t("settings.audit.table.requestId"),
                  render: (row) => (
                    <RowAction onClick={() => openDetail(row)}>
                      <code className="argus-mono">{row.id}</code>
                    </RowAction>
                  ),
                },
              ]}
              data={rows}
              getRowKey={(row) => row.id}
            />
          </div>
        )}
        <ActionGroup>
          <Button
            variant="secondary"
            disabled={!previous.length || events.isFetching}
            onClick={() => {
              setCursor(previous.at(-1));
              setPrevious(previous.slice(0, -1));
            }}
          >
            {t("settings.audit.previous")}
          </Button>
          <Button
            variant="secondary"
            disabled={
              !events.data?.hasMore ||
              !events.data.nextCursor ||
              events.isFetching
            }
            onClick={() => {
              setPrevious([...previous, cursor]);
              setCursor(events.data!.nextCursor!);
            }}
          >
            {t("settings.audit.next")}
          </Button>
        </ActionGroup>
      </div>

      <FormDrawer
        footer={
          <Button onClick={() => setSelected(null)} variant="secondary">
            {t("settings.common.close")}
          </Button>
        }
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        open={selected !== null}
        title={t("settings.audit.detailTitle")}
      >
        {selected && (
          <div className="argus-settings-stack">
            <KeyValueGrid
              columns={1}
              items={[
                {
                  label: t("settings.audit.detail.id"),
                  value: <code className="argus-mono">{selected.id}</code>,
                },
                {
                  label: t("settings.audit.detail.actor"),
                  value: actorLabel(selected),
                },
                {
                  label: t("settings.audit.detail.actorType"),
                  value: labelFor("actorTypes", selected.actorType),
                },
                {
                  label: t("settings.audit.detail.actorId"),
                  value: (
                    <code className="argus-mono">{selected.actorUserId}</code>
                  ),
                },
                {
                  label: t("settings.audit.detail.action"),
                  value: actionLabel(selected),
                },
                {
                  label: t("settings.audit.detail.actionKey"),
                  value: <code className="argus-mono">{selected.action}</code>,
                },
                {
                  label: t("settings.audit.detail.origin"),
                  value: t(`settings.audit.origins.${selected.origin}`),
                },
                {
                  label: t("settings.audit.detail.resourceType"),
                  value: selected.resourceType
                    ? labelFor("resourceTypes", selected.resourceType)
                    : "—",
                },
                {
                  label: t("settings.audit.detail.resourceName"),
                  value: selected.resourceName ?? "—",
                },
                {
                  label: t("settings.audit.detail.resourceId"),
                  value: selected.resourceId ?? "—",
                },
                {
                  label: t("settings.audit.detail.result"),
                  value: (
                    <StatusBadge tone={resultTone(selected.result)}>
                      {t(`settings.audit.results.${selected.result}`)}
                    </StatusBadge>
                  ),
                },
                {
                  label: t("settings.audit.detail.summary"),
                  value: summaryLabel(selected),
                },
                {
                  label: t("settings.audit.detail.createdAt"),
                  value: formatDateTime(selected.createdAt),
                },
                {
                  label: t("settings.audit.detail.eventHash"),
                  value: selected.eventHash ?? "—",
                },
                {
                  label: t("settings.audit.detail.previousHash"),
                  value: selected.previousHash ?? "—",
                },
              ]}
            />
            {selected.details && Object.keys(selected.details).length > 0 && (
              <section aria-label={t("settings.audit.detail.facts")}>
                <h3>{t("settings.audit.detail.facts")}</h3>
                <CodeBlock
                  code={JSON.stringify(selected.details, null, 2)}
                  language="json"
                />
              </section>
            )}
          </div>
        )}
      </FormDrawer>
    </PageShell>
  );
}
