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

export function BuiltinCollections({
  kind,
  id,
}: {
  kind: "host" | "kubernetes_cluster";
  id: string;
}) {
  const api = useApi(),
    { t } = useTranslation(),
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
    refetchInterval: 15000,
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
          {(
            [
              "host_process_metrics",
              "host_systemd_metrics",
              "host_windows_service_metrics",
              "kubernetes_scheduler_metrics",
              "kubernetes_controller_metrics",
              "kubernetes_etcd_metrics",
            ] as string[]
          ).includes(row.definition.key) && (
            <div className="argus-muted">
              {t(`builtinCollections.hints.${row.definition.key}`)}
            </div>
          )}
        </>
      ),
    },
    {
      key: "desired",
      header: t("monitoring.desiredEnabled"),
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
      key: "apply",
      header: t("monitoring.applicationState"),
      render: (row) =>
        row.collection ? (
          <>
            <Badge>
              {t(`monitoring.applyStates.${row.collection.apply_status}`)}
            </Badge>
            <div className="argus-muted">
              {t("monitoring.revisions", {
                desired: row.collection.desired_revision,
                effective: row.collection.effective_revision,
              })}
            </div>
          </>
        ) : (
          t("builtinCollections.notConfigured")
        ),
    },
    {
      key: "health",
      header: t("monitoring.collectionHealth"),
      render: (row) =>
        row.collection
          ? t(`monitoring.healthStates.${row.collection.health}`)
          : "—",
    },
    {
      key: "data",
      header: t("monitoring.dataState"),
      render: (row) =>
        row.collection
          ? t(`monitoring.freshnessStates.${row.collection.freshness}`)
          : "—",
    },
    {
      key: "reason",
      header: t("builtinCollections.availability"),
      render: (row) =>
        row.available
          ? row.collection?.error_code
            ? t(`builtinCollections.diagnostics.${row.collection.error_code}`, {
                defaultValue: row.collection.error_code,
              })
            : t("builtinCollections.available")
          : t(
              `builtinCollections.reasons.${row.reason ?? "component_unavailable"}`,
            ),
    },
    {
      key: "dashboard",
      header: t("monitoring.actions"),
      render: (row) => (
        <div className="argus-builtin-actions">
          {canManage &&
            row.available &&
            row.definition.component_tls_supported && (
              <Button size="sm" isDisabled={busy} onPress={() => setTrust(row)}>
                {t("builtinCollections.trust.action")}
              </Button>
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
      <h3 className="argus-detail-section__title">
        {t("builtinCollections.title")}
      </h3>
      <p className="argus-muted">{t("builtinCollections.hint")}</p>
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
