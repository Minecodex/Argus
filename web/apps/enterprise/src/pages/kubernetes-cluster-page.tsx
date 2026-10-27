import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import {
  Alert,
  Badge,
  EmptyState,
  PageShell,
  Spinner,
  StatusBadge,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import { CollectorStatusPanel } from "../components/kubernetes/collector-status";
import { CollectorWizard } from "../components/kubernetes/collector-wizard";
import { connectionStatusTone } from "../components/kubernetes/status";
import { KubernetesWorkbench } from "../components/kubernetes/workbench";
import { ResourceTelemetry } from "../components/telemetry/resource-telemetry";
import { ResourceDashboardLinks } from "../components/dashboards/resource-dashboard-links";
import { useResourceDashboardTab } from "../components/dashboards/use-resource-dashboard-tab";
import { usePermission } from "../lib/permissions";
import "../styles/kubernetes.css";

/** Kubernetes 集群详情：资源查询 + Collector 安装/管理。 */
export function KubernetesClusterPage() {
  const { t } = useTranslation();
  const api = useApi();
  const { clusterId } = useParams({ strict: false });
  const search = useSearch({ from: "/authed/admin/kubernetes/$clusterId" });
  const canReadDashboards = usePermission("telemetry.dashboard.read");
  const id = clusterId ?? "";
  const accessQuery = useQuery({
    queryKey: ["kubernetes-workbench", id, "scope"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.scope(id, signal),
    enabled: id.length > 0,
    refetchInterval: 10000,
  });

  const clusterQuery = useQuery({
    queryKey: ["kubernetes", "clusters", id],
    queryFn: () => api.kubernetes.getCluster(id),
    enabled: id.length > 0 && accessQuery.data?.cluster_wide === true,
  });
  const collectorQuery = useQuery({
    queryKey: ["kubernetes", "collector", id],
    queryFn: () => api.kubernetes.getCollector(id),
    enabled: id.length > 0 && accessQuery.data?.cluster_wide === true,
    refetchInterval: (query) =>
      query.state.data &&
      ["pending_install", "installing", "uninstalling"].includes(
        query.state.data.status,
      )
        ? 2000
        : false,
  });

  const cluster = clusterQuery.data;
  const initialTab =
    typeof window !== "undefined" && window.location.hash === "#otlp-collector"
      ? "collector"
      : "resources";
  const [tab, setTab] = useResourceDashboardTab(
    id,
    initialTab === "collector" ? undefined : search.tab,
    initialTab,
    canReadDashboards,
  );

  if (accessQuery.isPending || clusterQuery.isLoading) {
    return (
      <PageShell title={t("shell.nav.clusterDetail")}>
        <Spinner label={t("common.loading")} />
      </PageShell>
    );
  }
  if (
    accessQuery.data &&
    !accessQuery.error &&
    !accessQuery.data.cluster_wide
  ) {
    return (
      <PageShell
        title={accessQuery.data.cluster_name}
        breadcrumbs={[
          { label: t("shell.nav.kubernetes"), href: "/kubernetes" },
          { label: accessQuery.data.cluster_name },
        ]}
      >
        <KubernetesWorkbench
          key={id}
          clusterId={id}
          access={accessQuery.data}
        />
      </PageShell>
    );
  }

  if (!cluster || !accessQuery.data || accessQuery.error) {
    return (
      <PageShell
        breadcrumbs={[
          { label: t("shell.nav.kubernetes"), href: "/kubernetes" },
          { label: id },
        ]}
        title={t("shell.nav.clusterDetail")}
      >
        <EmptyState
          description={t("kubernetes.loadFailed")}
          kind="error"
          title={id}
        />
      </PageShell>
    );
  }

  return (
    <PageShell
      breadcrumbs={[
        { label: t("shell.nav.kubernetes"), href: "/kubernetes" },
        { label: cluster.name },
      ]}
      description={cluster.api_server}
      title={
        <span className="argus-k8s-cluster-card__head">
          {cluster.name}
          <Badge tone="accent">
            {t(`kubernetes.environment.${cluster.environment}`)}
          </Badge>
          <StatusBadge tone={connectionStatusTone(cluster.connection_status)}>
            {t(`kubernetes.status.${cluster.connection_status}`)}
          </StatusBadge>
          <Badge tone="neutral">{cluster.kubernetes_version}</Badge>
        </span>
      }
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="resources">
            {t("kubernetes.detail.resourcesTab")}
          </TabsTrigger>
          {canReadDashboards && (
            <TabsTrigger value="dashboards">
              {t("dashboardLinks.resourceTab")}
            </TabsTrigger>
          )}
          <TabsTrigger value="collector">
            {t("kubernetes.detail.collectorTab")}
          </TabsTrigger>
          <TabsTrigger value="metrics">{t("telemetry.metrics")}</TabsTrigger>
          <TabsTrigger value="logs">{t("telemetry.logs")}</TabsTrigger>
          <TabsTrigger value="traces">{t("telemetry.traces")}</TabsTrigger>
        </TabsList>
        <TabsContent value="resources">
          <KubernetesWorkbench
            key={id}
            clusterId={id}
            access={accessQuery.data}
          />
        </TabsContent>
        {canReadDashboards && (
          <TabsContent value="dashboards">
            <ResourceDashboardLinks type="kubernetes_cluster" id={id} />
          </TabsContent>
        )}
        <TabsContent value="collector">
          <div id="otlp-collector">
            {collectorQuery.isLoading ? (
              <Spinner label={t("common.loading")} />
            ) : collectorQuery.data ? (
              <CollectorStatusPanel
                cluster={cluster}
                collector={collectorQuery.data}
              />
            ) : (
              <div className="argus-k8s-stack">
                <Alert
                  description={t("kubernetes.collector.notInstalledHint")}
                  title={t("kubernetes.collector.notInstalled")}
                  tone="info"
                />
                <CollectorWizard cluster={cluster} />
              </div>
            )}
          </div>
        </TabsContent>
        <TabsContent value="metrics">
          <ResourceTelemetry
            resourceId={cluster.id}
            signal="metrics"
            resourceType="kubernetes_cluster"
          />
        </TabsContent>
        <TabsContent value="logs">
          <ResourceTelemetry
            resourceId={cluster.id}
            signal="logs"
            resourceType="kubernetes_cluster"
          />
        </TabsContent>
        <TabsContent value="traces">
          <ResourceTelemetry
            resourceId={cluster.id}
            signal="traces"
            resourceType="kubernetes_cluster"
          />
        </TabsContent>
      </Tabs>
    </PageShell>
  );
}
