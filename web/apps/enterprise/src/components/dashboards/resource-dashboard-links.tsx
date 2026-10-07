import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type DashboardResourceType,
  type DashboardBindingView,
  type DashboardItem,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  Dialog,
  EmptyState,
  ResourceCard,
  ResourceGrid,
  SearchInput,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { DashboardActionDialog } from "./action-dialog";
import "../../styles/dashboards.css";

export function ResourceDashboardLinks({
  type,
  id,
}: {
  type: DashboardResourceType;
  id: string;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const canRead = usePermission("telemetry.dashboard.read"),
    canManage = usePermission(
      type === "host" ? "host.manage" : "kubernetes.manage",
    );
  const [open, setOpen] = useState(false),
    [search, setSearch] = useState("");
  const [action, setAction] = useState<PendingActionPublic | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const bindings = useQuery({
    queryKey: ["dashboard-bindings", type, id],
    queryFn: () => api.dashboards.resourceBindings(type, id),
    enabled: canRead && Boolean(id),
    refetchOnWindowFocus: true,
  });
  const available = useQuery({
    queryKey: ["dashboard-binding-candidates", type, id],
    queryFn: () => api.dashboards.list(),
    enabled: canRead && canManage && open,
    staleTime: 0,
  });
  const linked = bindings.data?.items ?? [];
  const candidates = (available.data ?? []).filter(
    (d) =>
      d.lifecycle === "active" &&
      !linked.some((binding) => binding.dashboard.id === d.id),
  );
  const match = (name: string) =>
    name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase());
  const failed = (cause: unknown) => {
    setError(
      formatApiError(cause, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      ),
    );
  };
  const preview = async (
    dashboard: DashboardItem,
    binding?: DashboardBindingView,
  ) => {
    if (!canManage || !bindings.data) return;
    setBusy(true);
    setError("");
    try {
      setAction(
        await api.dashboards.previewBinding(type, id, {
          operation: binding ? "detach" : "attach",
          dashboard_id: dashboard.id,
          expected_dashboard_version: dashboard.version,
          expected_resource_version: bindings.data.resource_version,
          ...(binding
            ? {
                binding_id: binding.id,
                expected_binding_version: binding.version,
              }
            : {}),
        }),
      );
    } catch (e) {
      failed(e);
      void bindings.refetch();
      void available.refetch();
    } finally {
      setBusy(false);
    }
  };
  const openDashboard = (dashboardId: string) =>
    void navigate({
      to: "/dashboards/$dashboardId",
      params: { dashboardId },
      search: { resource: id, resource_type: type },
    });
  const failure = (cause: unknown) => (
    <Alert
      tone="danger"
      title={t("dashboardLinks.linkingFailed")}
      description={formatApiError(cause, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      )}
    />
  );
  if (!canRead) return null;
  return (
    <section
      className="argus-dashboard-resource-tab"
      aria-label={t("dashboards.resourceLinks")}
    >
      <div className="argus-dashboard-resource-tab__heading">
        <div className="argus-dashboard-inline">
          <h2>{t("dashboards.resourceLinks")}</h2>
          {bindings.data && !bindings.error && !bindings.isFetching && (
            <Badge>{linked.length}</Badge>
          )}
        </div>
        {canManage ? (
          <Button
            onPress={() => {
              setOpen(true);
              setSearch("");
              setError("");
              void bindings.refetch();
            }}
          >
            {t("dashboards.manageBindings")}
          </Button>
        ) : (
          <Badge>{t("dashboardLinks.readOnly")}</Badge>
        )}
      </div>
      <p className="argus-dashboard-muted">
        {t("dashboardLinks.resourceHint")}
      </p>
      {bindings.isFetching ? (
        <p role="status">{t("common.loading")}</p>
      ) : bindings.error ? (
        <>
          {failure(bindings.error)}
          <Button onPress={() => void bindings.refetch()}>
            {t("dashboardLinks.retry")}
          </Button>
        </>
      ) : bindings.data ? (
        linked.length ? (
          <ResourceGrid>
            {linked.map((binding) => (
              <ResourceCard
                key={binding.id}
                title={
                  <Link
                    className="argus-dashboard-resource-title"
                    to="/dashboards/$dashboardId"
                    params={{ dashboardId: binding.dashboard.id }}
                    search={{ resource: id, resource_type: type }}
                  >
                    {binding.dashboard.name}
                  </Link>
                }
                subtitle={binding.dashboard.description || undefined}
                status={<Badge>{t("dashboardLinks.published")}</Badge>}
                actions={
                  <Button onPress={() => openDashboard(binding.dashboard.id)}>
                    {t("dashboardLinks.openDashboard")}
                  </Button>
                }
              />
            ))}
          </ResourceGrid>
        ) : (
          <EmptyState
            title={t("dashboardLinks.noDashboards")}
            description={t("dashboardLinks.noDashboardsHint")}
          />
        )
      ) : (
        <p role="status">{t("common.loading")}</p>
      )}
      <Dialog
        open={open && canManage}
        onOpenChange={setOpen}
        title={t("dashboards.manageBindings")}
        description={t("dashboardLinks.manageHint")}
        size="lg"
      >
        <div className="argus-dashboard-form-stack">
          <SearchInput
            aria-label={t("dashboardLinks.search")}
            placeholder={t("dashboardLinks.search")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {error && (
            <Alert
              tone="danger"
              title={t("dashboards.failed")}
              description={error}
            />
          )}
          {bindings.error ? (
            failure(bindings.error)
          ) : bindings.isFetching ? (
            <p role="status">{t("common.loading")}</p>
          ) : (
            linked
              .filter((binding) => match(binding.dashboard.name))
              .map((binding) => (
                <div className="argus-dashboard-binding-row" key={binding.id}>
                  <div className="argus-dashboard-binding-row__identity">
                    <strong>{binding.dashboard.name}</strong>
                    <Badge>{t("dashboardLinks.linked")}</Badge>
                  </div>
                  <Button
                    isDisabled={busy}
                    aria-label={t("dashboardLinks.detachLabel", {
                      name: binding.dashboard.name,
                    })}
                    onPress={() => void preview(binding.dashboard, binding)}
                  >
                    {t("dashboardLinks.detach")}
                  </Button>
                </div>
              ))
          )}
          {available.error ? (
            <>
              {failure(available.error)}
              <Button onPress={() => void available.refetch()}>
                {t("dashboardLinks.retry")}
              </Button>
            </>
          ) : available.isFetching ? (
            <p role="status">{t("common.loading")}</p>
          ) : (
            candidates
              .filter((d) => match(d.name))
              .map((d) => (
                <div className="argus-dashboard-binding-row" key={d.id}>
                  <div className="argus-dashboard-binding-row__identity">
                    <strong>{d.name}</strong>
                    <Badge>{t("dashboardLinks.unlinked")}</Badge>
                  </div>
                  <Button
                    variant="primary"
                    isDisabled={
                      busy ||
                      bindings.isFetching ||
                      Boolean(bindings.error) ||
                      !bindings.data
                    }
                    aria-label={t("dashboardLinks.attachLabel", {
                      name: d.name,
                    })}
                    onPress={() => void preview(d)}
                  >
                    {t("dashboardLinks.attach")}
                  </Button>
                </div>
              ))
          )}
          {!bindings.error &&
            !available.error &&
            !bindings.isFetching &&
            !available.isFetching &&
            ![
              ...linked.map((binding) => binding.dashboard),
              ...candidates,
            ].some((d) => match(d.name)) && (
              <EmptyState
                title={t("dashboardLinks.noCandidates")}
                description={t("dashboardLinks.noCandidatesHint")}
              />
            )}
        </div>
      </Dialog>
      {action && canManage && (
        <DashboardActionDialog
          action={action}
          onClose={() => setAction(null)}
          onError={(e) => {
            failed(e);
            setAction(null);
            void bindings.refetch();
            void available.refetch();
            return true;
          }}
          onDone={() => {
            setAction(null);
            void cache.invalidateQueries({ queryKey: ["dashboard-bindings"] });
            void available.refetch();
          }}
        />
      )}
    </section>
  );
}
