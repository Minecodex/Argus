import { useState } from "react";
import "../../styles/monitoring.css";
import { useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ComponentTrustDialog } from "./component-trust-dialog";
import {
  useApi,
  formatApiError,
  type MonitoringSchemas,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  DataTable,
  ActionMenu,
  Switch,
  Badge,
  Dialog,
  EmptyState,
  Button,
  type Column,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";

const builtinTemplates: Record<string, string> = {
  host_metrics: "host.overview",
  host_process_metrics: "host.processes",
  host_systemd_metrics: "host.systemd",
  host_windows_service_metrics: "host.windows_services",
  collector_metrics: "collector.overview",
  kubernetes_cluster_metrics: "kubernetes.cluster",
  kubernetes_node_metrics: "kubernetes.nodes",
  kubernetes_apiserver_metrics: "kubernetes.apiserver",
  kubernetes_coredns_metrics: "kubernetes.coredns",
  kubernetes_scheduler_metrics: "kubernetes.scheduler",
  kubernetes_controller_metrics: "kubernetes.controller_manager",
  kubernetes_etcd_metrics: "kubernetes.etcd",
};
const applyingStates = new Set(["pending", "applying", "unknown"]);

export function BuiltinCollections({
  kind,
  id,
  showTitle = true,
}: {
  kind: "host" | "kubernetes_cluster";
  id: string;
  showTitle?: boolean;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    cache = useQueryClient();
  const navigate = useNavigate(),
    canViewDashboard = usePermission("telemetry.dashboard.read");
  const openDefault = async (key: string) => {
    try {
      const template = builtinTemplates[key];
      if (!template)
        throw new Error(t("monitoring.defaultDashboardUnavailable"));
      const item = (await api.dashboards.list()).find(
        (item) => item.managed_template_key === template,
      );
      if (!item) throw new Error(t("monitoring.defaultDashboardUnavailable"));
      await navigate({
        to: "/dashboards/$dashboardId",
        params: { dashboardId: item.id },
        search: { resource: id, resource_type: kind },
      });
    } catch (error) {
      setError(message(error));
    }
  };
  const canRead = usePermission("telemetry.collector.read"),
    canManage = usePermission("telemetry.collector.manage");
  const [pending, setPending] = useState<PendingActionPublic>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [trust, setTrust] = useState<MonitoringSchemas["MonitoringBuiltin"]>();
  const query = useQuery({
    queryKey: ["monitoring", "builtins", kind, id],
    queryFn: () => api.monitoring.builtins({ kind, id }),
    enabled: canRead,
    refetchInterval: (query) =>
      query.state.data?.some((row) =>
        applyingStates.has(row.collection?.apply_status ?? ""),
      )
        ? 2000
        : 15000,
  });
  const message = (error: unknown) =>
    formatApiError(error, t("monitoring.errors.unavailable"), (requestId) =>
      t("common.requestReference", { requestId }),
    );
  const toggle = async (
    row: MonitoringSchemas["MonitoringBuiltin"],
    enabled: boolean,
  ) => {
    setBusy(true);
    setError("");
    try {
      setPending(
        await api.monitoring.previewBuiltin({
          target: { kind, id },
          key: row.definition.key,
          expected_version: row.collection?.version ?? 0,
          enabled,
        }),
      );
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  };
  const columns: Column<MonitoringSchemas["MonitoringBuiltin"]>[] = [
    {
      key: "name",
      header: t("builtinCollections.name"),
      render: (row) => (
        <>
          <div>{t(`builtinCollections.names.${row.definition.key}`)}</div>
          <div className="argus-muted">
            {t(`builtinCollections.descriptions.${row.definition.key}`)}
          </div>
          {!row.available && (
            <div className="argus-muted">
              {t(
                `builtinCollections.reasons.${row.reason ?? "component_unavailable"}`,
              )}
            </div>
          )}
        </>
      ),
    },
    {
      key: "desired",
      header: t("builtinCollections.enable"),
      render: (row) => (
        <Switch
          label={t("monitoring.toggleCollection", {
            name: t(`builtinCollections.names.${row.definition.key}`),
          })}
          checked={row.collection?.desired_enabled ?? false}
          disabled={!canManage || !row.available || busy}
          onChange={(enabled) => void toggle(row, enabled)}
        />
      ),
    },
    {
      key: "data",
      header: t("monitoring.dataState"),
      render: (row) => {
        const collection = row.collection;
        const applying =
          collection && applyingStates.has(collection.apply_status);
        const enabled = collection?.desired_enabled;
        const failed =
          enabled &&
          (collection?.apply_status === "failed" ||
            collection?.health === "failed");
        const warning =
          enabled &&
          (collection?.freshness === "stale" ||
            collection?.health === "degraded");
        const label = applying
          ? t(`monitoring.applyStates.${collection.apply_status}`)
          : failed
            ? t("monitoring.healthStates.failed")
            : !enabled
              ? t("builtinCollections.off")
              : collection.health === "degraded"
                ? t("monitoring.healthStates.degraded")
                : t(`monitoring.freshnessStates.${collection.freshness}`);
        return (
          <>
            <Badge
              tone={
                applying
                  ? "accent"
                  : failed
                    ? "danger"
                    : warning
                      ? "warning"
                      : enabled && collection?.freshness === "fresh"
                        ? "success"
                        : "neutral"
              }
            >
              {label}
            </Badge>
            {enabled && collection?.last_sample_at && (
              <div className="argus-muted">
                {t("builtinCollections.latestSample", {
                  time: new Date(collection.last_sample_at).toLocaleString(
                    i18n.language,
                  ),
                })}
              </div>
            )}
            {enabled &&
              collection?.error_code &&
              [
                "COLLECTION_SCRAPE_FAILED",
                "COLLECTION_SCRAPE_PARTIAL_FAILURE",
              ].includes(collection.error_code) && (
                <div className="argus-muted">
                  {t(`builtinCollections.diagnostics.${collection.error_code}`)}
                </div>
              )}
          </>
        );
      },
    },
    {
      key: "dashboard",
      header: t("monitoring.actions"),
      render: (row) => (
        <div className="argus-builtin-actions">
          {canManage &&
            row.available &&
            row.definition.component_tls_supported && (
              <ActionMenu
                name={t(`builtinCollections.names.${row.definition.key}`)}
                isDisabled={busy}
                items={[
                  {
                    label: t("builtinCollections.trust.action"),
                    onSelect: () => setTrust(row),
                  },
                ]}
              />
            )}
          {canViewDashboard &&
          row.collection &&
          row.available &&
          builtinTemplates[row.definition.key] ? (
            <Button
              size="sm"
              onPress={() => void openDefault(row.definition.key)}
            >
              {t("monitoring.openDefaultDashboard")}
            </Button>
          ) : null}
        </div>
      ),
    },
  ];
  return (
    <section className="argus-detail-section">
      {showTitle && (
        <h3 className="argus-detail-section__title">
          {t("builtinCollections.title")}
        </h3>
      )}
      {showTitle && (
        <p className="argus-muted">{t("builtinCollections.hint")}</p>
      )}
      {!canRead ? (
        <EmptyState
          title={t("builtinCollections.noAccess")}
          description={t("monitoring.errors.denied")}
        />
      ) : query.isPending ? (
        <p role="status">{t("common.loading")}</p>
      ) : query.error ? (
        <p role="alert">{message(query.error)}</p>
      ) : (
        <DataTable
          data={query.data ?? []}
          columns={columns}
          getRowKey={(row) => row.id}
        />
      )}
      {error && <p role="alert">{error}</p>}
      {trust && (
        <ComponentTrustDialog
          row={trust}
          kind={kind}
          id={id}
          onClose={() => setTrust(undefined)}
          onPreview={(action) => {
            setTrust(undefined);
            setPending(action);
          }}
        />
      )}
      {pending && (
        <Dialog
          open
          title={t("builtinCollections.preview")}
          onOpenChange={(open) => {
            if (!open) setPending(undefined);
          }}
        >
          <PendingActionConfirm
            action={pending}
            onCancel={() => setPending(undefined)}
            onDismiss={() => setPending(undefined)}
            onDone={async () => {
              setBusy(true);
              try {
                await Promise.all([
                  cache.invalidateQueries({
                    queryKey: ["monitoring", "builtins", kind, id],
                  }),
                  cache.invalidateQueries({ queryKey: ["telemetry"] }),
                ]);
                setPending(undefined);
              } finally {
                setBusy(false);
              }
            }}
          />
        </Dialog>
      )}
    </section>
  );
}
