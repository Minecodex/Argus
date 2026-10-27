import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  HardDrive,
  Layers3,
  MoreHorizontal,
  Component,
  Database,
} from "lucide-react";
import { formatApiError, useApi } from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  DrawerPanel,
  Dropdown,
  EmptyState,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";

export function DashboardLinkedResources({ id }: { id: string }) {
  const api = useApi(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const canReadHosts = usePermission("host.read"),
    canReadClusters = usePermission("kubernetes.read"),
    canReadApplications = usePermission("application.read");
  const query = useQuery({
    queryKey: ["dashboard-bindings", "dashboard", id],
    queryFn: () => api.dashboards.bindings(id),
    enabled: open,
    refetchOnWindowFocus: true,
  });
  return (
    <>
      <Dropdown
        trigger={
          <Button isIconOnly aria-label={t("dashboardLinks.more")}>
            <MoreHorizontal aria-hidden />
          </Button>
        }
        items={[
          {
            label: t("dashboardLinks.linkedResources"),
            onSelect: () => setOpen(true),
          },
        ]}
      />
      <DrawerPanel
        open={open}
        onOpenChange={setOpen}
        title={t("dashboardLinks.linkedResources")}
        description={t("dashboardLinks.reverseHint")}
        footer={
          canReadHosts || canReadClusters ? (
            <>
              {canReadHosts && (
                <Button
                  onPress={() => {
                    setOpen(false);
                    void navigate({ to: "/hosts" });
                  }}
                >
                  {t("dashboardLinks.browseHosts")}
                </Button>
              )}
              {canReadClusters && (
                <Button
                  onPress={() => {
                    setOpen(false);
                    void navigate({ to: "/kubernetes" });
                  }}
                >
                  {t("dashboardLinks.browseClusters")}
                </Button>
              )}
            </>
          ) : undefined
        }
      >
        <div className="argus-dashboard-form-stack">
          {query.isFetching ? (
            <p role="status">{t("common.loading")}</p>
          ) : query.error ? (
            <>
              <Alert
                tone="danger"
                title={t("dashboardLinks.loadFailed")}
                description={formatApiError(
                  query.error,
                  t("dashboards.failed"),
                  (requestId) => t("common.requestReference", { requestId }),
                )}
              />
              <Button onPress={() => void query.refetch()}>
                {t("dashboardLinks.retry")}
              </Button>
            </>
          ) : query.data ? (
            <>
              <div className="argus-dashboard-inline">
                <Badge>{query.data.length}</Badge>
                <span className="argus-dashboard-muted">
                  {t("dashboardLinks.authorizedLinks")}
                </span>
              </div>
              {query.data.map((binding) => (
                <div
                  key={binding.id}
                  className="argus-dashboard-linked-resource"
                >
                  <span className="argus-dashboard-linked-resource__icon">
                    {binding.resource_type === "host" ? (
                      <HardDrive aria-hidden />
                    ) : binding.resource_type === "kubernetes_cluster" ? (
                      <Layers3 aria-hidden />
                    ) : binding.resource_type === "application" ? (
                      <Component aria-hidden />
                    ) : (
                      <Database aria-hidden />
                    )}
                  </span>
                  <div className="argus-dashboard-linked-resource__identity">
                    <strong>{binding.resource_name}</strong>
                    <span className="argus-dashboard-muted">
                      {t(
                        {
                          host: "dashboardLinks.host",
                          kubernetes_cluster: "dashboardLinks.cluster",
                          application: "dashboardLinks.application",
                          monitoring_target: "dashboardLinks.monitoringTarget",
                        }[binding.resource_type],
                      )}
                    </span>
                  </div>
                  {binding.resource_type === "host"
                    ? canReadHosts && (
                        <Link
                          className="argus-dashboard-resource-link"
                          to="/hosts/$hostId"
                          params={{ hostId: binding.resource_id }}
                          search={{ tab: "dashboards" }}
                          onClick={() => setOpen(false)}
                        >
                          {t("dashboardLinks.openResource")}
                        </Link>
                      )
                    : binding.resource_type === "kubernetes_cluster" &&
                      canReadClusters && (
                        <Link
                          className="argus-dashboard-resource-link"
                          to="/kubernetes/$clusterId"
                          params={{ clusterId: binding.resource_id }}
                          search={{ tab: "dashboards" }}
                          onClick={() => setOpen(false)}
                        >
                          {t("dashboardLinks.openResource")}
                        </Link>
                      )}
                  {binding.resource_type === "application" &&
                    canReadApplications && (
                      <Link
                        className="argus-dashboard-resource-link"
                        to="/applications/$applicationId"
                        params={{ applicationId: binding.resource_id }}
                        onClick={() => setOpen(false)}
                      >
                        {t("dashboardLinks.openResource")}
                      </Link>
                    )}
                </div>
              ))}
              {!query.data.length && (
                <EmptyState
                  title={t("dashboardLinks.noLinks")}
                  description={t("dashboardLinks.noLinksHint")}
                />
              )}
            </>
          ) : (
            <p role="status">{t("common.loading")}</p>
          )}
          <p className="argus-dashboard-muted">
            {t("dashboards.resourcesNote")}
          </p>
        </div>
      </DrawerPanel>
    </>
  );
}
