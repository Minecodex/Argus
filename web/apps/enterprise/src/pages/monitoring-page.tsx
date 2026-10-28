import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import {
  formatApiError,
  useApi,
  managedDashboardTemplates,
  builtinCollections,
  type MonitoringCollection,
  type MonitoringIntegration,
} from "@argus/api-client";
import {
  Badge,
  Button,
  Pagination,
  DataTable,
  EmptyState,
  PageShell,
  Switch,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  type Column,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { useMonitoringActions } from "../components/monitoring/actions";
import {
  CollectionFilters,
  type CollectionFilterValue,
} from "../components/monitoring/collection-filters";
import { CollectionForm } from "../components/monitoring/collection-form";
import { LogCollectionForm } from "../components/monitoring/log-collection-form";
import {
  IntegrationCatalog,
  IntegrationPicker,
} from "../components/monitoring/integration-catalog";
import "../styles/monitoring.css";

export function MonitoringPage() {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    actions = useMonitoringActions();
  const canRead = usePermission("monitoring.read"),
    canManage = usePermission("monitoring.manage");
  const canReadDashboards = usePermission("telemetry.dashboard.read"),
    navigate = useNavigate();
  const [dashboardError, setDashboardError] = useState("");
  const openDashboard = async (item: MonitoringCollection) => {
    try {
      setDashboardError("");
      const key = managedDashboardTemplates.find((template) =>
        template.integration_keys.some((key) => key === item.integration),
      )?.key;
      const dashboard = (await api.dashboards.list()).find(
        (row) => row.managed_template_key === key,
      );
      if (!dashboard) {
        setDashboardError(t("monitoring.defaultDashboardUnavailable"));
        return;
      }
      await navigate({
        to: "/dashboards/$dashboardId",
        params: { dashboardId: dashboard.id },
        search: {
          resource: item.target.id,
          resource_type: item.target.kind,
        },
      });
    } catch (error) {
      setDashboardError(
        formatApiError(error, t("monitoring.errors.unavailable"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    }
  };
  const [tab, setTab] = useState("collections"),
    [search, setSearch] = useState(""),
    [filters, setFilters] = useState<CollectionFilterValue>({}),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const [choosing, setChoosing] = useState(false),
    [creating, setCreating] = useState<MonitoringIntegration>();
  const catalog = useQuery({
    queryKey: ["monitoring", "integrations"],
    queryFn: () => api.monitoring.integrations(),
    enabled: canRead,
  });
  const query = useQuery({
    queryKey: ["monitoring", "collections", search, filters, cursors.at(-1)],
    queryFn: () =>
      api.monitoring.collections({
        ...filters,
        search,
        cursor: cursors.at(-1),
        limit: 30,
      }),
    enabled: canRead,
  });
  const hasFilters =
    search.trim() !== "" ||
    Object.values(filters).some((value) => value !== undefined && value !== "");
  const columns: Column<MonitoringCollection>[] = [
    { key: "name", header: t("monitoring.collectionName") },
    {
      key: "integration",
      header: t("monitoring.integration"),
      render: (item) =>
        builtinCollections.some((entry) => entry.key === item.integration)
          ? t(`builtinCollections.names.${item.integration}`)
          : (catalog.data?.find((entry) => entry.key === item.integration)
              ?.name ?? item.integration),
    },
    {
      key: "desired_enabled",
      header: t("monitoring.desiredEnabled"),
      render: (item) => (
        <Switch
          label={t("monitoring.toggleCollection", { name: item.name })}
          checked={item.desired_enabled}
          disabled={!canManage || actions.busy}
          onChange={(enabled) =>
            void actions.preview({
              kind: "collection",
              operation: "set_enabled",
              id: item.id,
              expected_version: item.version,
              collection: { enabled },
            })
          }
        />
      ),
    },
    {
      key: "apply_status",
      header: t("monitoring.applicationState"),
      render: (item) => (
        <div>
          <Badge>{t(`monitoring.applyStates.${item.apply_status}`)}</Badge>
          <div className="argus-muted">
            {t("monitoring.revisions", {
              effective: item.effective_revision,
              desired: item.desired_revision,
            })}
          </div>
          {item.effective_collector_id &&
            item.effective_collector_id !== item.collector_id && (
              <div className="argus-muted">
                {t("monitoring.previousExecutionActive")}
              </div>
            )}
        </div>
      ),
    },
    {
      key: "health",
      header: t("monitoring.collectionHealth"),
      render: (item) => t(`monitoring.healthStates.${item.health}`),
    },
    {
      key: "freshness",
      header: t("monitoring.dataState"),
      render: (item) => t(`monitoring.freshnessStates.${item.freshness}`),
    },
    {
      key: "last_sample_at",
      header: t("monitoring.lastSample"),
      render: (item) =>
        item.last_sample_at
          ? new Intl.DateTimeFormat(i18n.language, {
              dateStyle: "short",
              timeStyle: "medium",
            }).format(new Date(item.last_sample_at))
          : "—",
    },
    {
      key: "actions",
      header: t("monitoring.actions"),
      render: (item) => (
        <div className="argus-monitoring-actions">
          {canReadDashboards &&
            managedDashboardTemplates.some((template) =>
              template.integration_keys.some((key) => key === item.integration),
            ) && (
              <Button size="sm" onPress={() => void openDashboard(item)}>
                {t("monitoring.openDefaultDashboard")}
              </Button>
            )}
          <Button
            size="sm"
            isDisabled={!canManage || actions.busy}
            onPress={() =>
              void actions.preview({
                kind: "collection",
                operation: "archive",
                id: item.id,
                expected_version: item.version,
                collection: {},
              })
            }
          >
            {t("monitoring.archiveCollection")}
          </Button>
        </div>
      ),
    },
  ];
  return (
    <PageShell
      title={t("monitoring.title")}
      description={t("monitoring.workspaceHint")}
      actions={
        canManage && (
          <Button variant="primary" onPress={() => setChoosing(true)}>
            <Plus size={16} />
            {t("monitoring.createCollection")}
          </Button>
        )
      }
    >
      {!canRead ? (
        <EmptyState
          title={t("monitoring.noMonitoringAccess")}
          description={t("monitoring.errors.denied")}
        />
      ) : (
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="collections">
              {t("monitoring.collections")}
            </TabsTrigger>
            <TabsTrigger value="integrations">
              {t("monitoring.integrationCatalog")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="collections">
            <CollectionFilters
              value={filters}
              search={search}
              integrations={catalog.data ?? []}
              onChange={(value) => {
                setFilters(value);
                setCursors([undefined]);
              }}
              onSearch={(value) => {
                setSearch(value);
                setCursors([undefined]);
              }}
              onRefresh={() => void query.refetch()}
              refreshing={query.isFetching}
            />
            {query.isPending && <p role="status">{t("common.loading")}</p>}
            {query.error && (
              <p role="alert">
                {formatApiError(
                  query.error,
                  t("monitoring.errors.unavailable"),
                  (requestId) => t("common.requestReference", { requestId }),
                )}
              </p>
            )}
            {query.data?.items.length ? (
              <DataTable
                data={query.data.items}
                columns={columns}
                getRowKey={(item) => item.id}
              />
            ) : (
              !query.isPending &&
              !query.error && (
                <EmptyState
                  title={t(
                    hasFilters
                      ? "monitoring.noFilterMatches"
                      : "monitoring.noCollections",
                  )}
                  description={t(
                    hasFilters
                      ? "monitoring.noFilterMatchesHint"
                      : "monitoring.noCollectionsHint",
                  )}
                />
              )
            )}
            <Pagination
              page={cursors.length}
              hasNext={!!query.data?.next_cursor}
              isDisabled={query.isFetching}
              onChange={(page) =>
                setCursors((old) =>
                  page <= old.length
                    ? old.slice(0, page)
                    : [...old, query.data?.next_cursor],
                )
              }
            />
          </TabsContent>
          <TabsContent value="integrations">
            {catalog.data && (
              <IntegrationCatalog
                items={catalog.data}
                canCreate={canManage}
                onCreate={setCreating}
              />
            )}
          </TabsContent>
        </Tabs>
      )}
      {actions.error && <p role="alert">{actions.error}</p>}
      {dashboardError && <p role="alert">{dashboardError}</p>}
      {choosing && (
        <IntegrationPicker
          items={catalog.data ?? []}
          onSelect={(entry) => {
            setChoosing(false);
            setCreating(entry);
          }}
          onClose={() => setChoosing(false)}
        />
      )}
      {creating &&
      ["filelog", "journald", "eventlog", "k8slog"].includes(creating.key) ? (
        <LogCollectionForm
          integration={creating}
          busy={actions.busy}
          error={actions.error}
          onSubmit={actions.preview}
          onClose={() => setCreating(undefined)}
        />
      ) : (
        creating && (
          <CollectionForm
            integration={creating}
            busy={actions.busy}
            error={actions.error}
            onSubmit={actions.preview}
            onClose={() => setCreating(undefined)}
          />
        )
      )}
      {actions.confirmation}
    </PageShell>
  );
}
