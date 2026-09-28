import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type DashboardResourceType,
  type DashboardBindingView,
  type PendingActionPublic,
} from "@argus/api-client";
import { Alert, Button, Dialog, Select } from "@argus/ui";
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
    { t } = useTranslation();
  const canRead = usePermission("telemetry.dashboard.read"),
    canManage = usePermission(
      type === "host" ? "host.manage" : "kubernetes.manage",
    );
  const [open, setOpen] = useState(false),
    [choice, setChoice] = useState(""),
    [action, setAction] = useState<PendingActionPublic | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const key = ["dashboard-bindings", type, id];
  const bindings = useQuery({
    queryKey: key,
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
  const candidates = (available.data ?? []).filter(
    (d) =>
      d.lifecycle === "active" &&
      !bindings.data?.items.some((b) => b.dashboard.id === d.id),
  );
  const failure = (cause: unknown) =>
    setError(
      formatApiError(cause, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      ),
    );
  const preview = async (binding?: DashboardBindingView) => {
    const selected =
      binding?.dashboard ?? candidates.find((d) => d.id === choice);
    if (!selected || !bindings.data) return;
    setBusy(true);
    setError("");
    try {
      const next = await api.dashboards.previewBinding(type, id, {
        operation: binding ? "detach" : "attach",
        dashboard_id: selected.id,
        expected_dashboard_version: selected.version,
        expected_resource_version: bindings.data.resource_version,
        ...(binding
          ? {
              binding_id: binding.id,
              expected_binding_version: binding.version,
            }
          : {}),
      });
      setAction(next);
    } catch (e) {
      failure(e);
      void bindings.refetch();
      void available.refetch();
    } finally {
      setBusy(false);
    }
  };
  if (!canRead) return null;
  return (
    <section
      className="argus-dashboard-resource-links"
      aria-label={t("dashboards.resourceLinks")}
    >
      <div className="argus-dashboard-inline">
        <strong>{t("dashboards.resourceLinks")}</strong>
        {bindings.data?.items.map((binding) => (
          <Link
            className="argus-dashboard-shortcut"
            key={binding.id}
            to="/dashboards/$dashboardId"
            params={{ dashboardId: binding.dashboard.id }}
            search={{ resource: id }}
          >
            {binding.dashboard.name}
          </Link>
        ))}
        {canManage && (
          <Button
            size="sm"
            onClick={() => {
              setOpen(true);
              setChoice("");
              setError("");
              void bindings.refetch();
            }}
          >
            {t("dashboards.manageBindings")}
          </Button>
        )}
        {!bindings.isLoading &&
          !bindings.error &&
          !bindings.data?.items.length && (
            <span className="argus-dashboard-muted">
              {t("dashboards.noBindings")}
            </span>
          )}
      </div>
      {bindings.error && (
        <p role="alert">
          {formatApiError(bindings.error, t("dashboards.failed"), (requestId) =>
            t("common.requestReference", { requestId }),
          )}
        </p>
      )}
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t("dashboards.manageBindings")}
        size="lg"
      >
        <div className="argus-dashboard-form-stack">
          <p>{t("dashboards.resourcesNote")}</p>
          {error && (
            <Alert
              tone="danger"
              title={t("dashboards.failed")}
              description={error}
            />
          )}
          {available.error && (
            <p role="alert">
              {formatApiError(
                available.error,
                t("dashboards.failed"),
                (requestId) => t("common.requestReference", { requestId }),
              )}
            </p>
          )}
          {bindings.data?.items.map((binding) => (
            <div className="argus-dashboard-inline" key={binding.id}>
              <span>{binding.dashboard.name}</span>
              <Button
                size="sm"
                disabled={busy}
                onClick={() => void preview(binding)}
              >
                {t("dashboards.detachBinding")}
              </Button>
            </div>
          ))}
          <div className="argus-dashboard-inline">
            <Select
              ariaLabel={t("dashboards.chooseDashboard")}
              value={choice}
              onValueChange={setChoice}
              options={[
                { value: "", label: t("dashboards.chooseDashboard") },
                ...candidates.map((d) => ({ value: d.id, label: d.name })),
              ]}
            />
            <Button
              disabled={
                busy || !choice || bindings.isFetching || available.isFetching
              }
              onClick={() => void preview()}
            >
              {t("dashboards.attachBinding")}
            </Button>
          </div>
        </div>
      </Dialog>
      {action && (
        <DashboardActionDialog
          action={action}
          onClose={() => setAction(null)}
          onError={(e) => {
            failure(e);
            setAction(null);
            void bindings.refetch();
            void available.refetch();
            return true;
          }}
          onDone={() => {
            setAction(null);
            setChoice("");
            void cache.invalidateQueries({ queryKey: ["dashboard-bindings"] });
            void available.refetch();
          }}
        />
      )}
    </section>
  );
}
