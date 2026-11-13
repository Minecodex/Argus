import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import {
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
import { KubernetesClusterOverview } from "../components/kubernetes/cluster-overview";
import { KubernetesOperationsDialog } from "../components/kubernetes/operations-dialog";
import { ResourceObserveLink } from "../components/observe/resource-link";
import { ResourceDashboardLinks } from "../components/dashboards/resource-dashboard-links";
import { useResourceDashboardTab } from "../components/dashboards/use-resource-dashboard-tab";
import { useResourceLocation } from "../components/kubernetes/resource-location";
import { usePermission } from "../lib/permissions";
import { ResourceExecutionActivity } from "../components/operations/execution-result";
import "../styles/kubernetes.css";
import "../styles/kubernetes-workbench.css";

export function KubernetesClusterPage() {
  const { t } = useTranslation(),
    api = useApi(),
    { clusterId } = useParams({ strict: false }),
    id = clusterId ?? "";
  const search = useSearch({ from: "/authed/admin/kubernetes/$clusterId" }),
    location = useResourceLocation(id);
  const canReadDashboards = usePermission("telemetry.dashboard.read");
  const access = useQuery({
    queryKey: ["kubernetes-workbench", id, "scope"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.scope(id, signal),
    enabled: !!id,
    refetchInterval: 10000,
  });
  const wide = access.data?.cluster_wide === true;
  const cluster = useQuery({
    queryKey: ["kubernetes", "clusters", id],
    queryFn: () => api.kubernetes.getCluster(id),
    enabled: !!id && wide,
  });
  const collector = useQuery({
    queryKey: ["kubernetes", "collector", id],
    queryFn: () => api.kubernetes.getCollector(id),
    enabled: !!id && wide,
    refetchInterval: (q) =>
      q.state.data &&
      ["pending_install", "installing", "uninstalling"].includes(
        q.state.data.status,
      )
        ? 2000
        : false,
  });
  const allowed = [
    "resources",
    "activity",
    ...(canReadDashboards ? ["overview"] : []),
    ...(wide
      ? ["collector", ...(canReadDashboards ? ["dashboards"] : [])]
      : []),
  ];
  const initial =
    window.location.hash === "#otlp-collector" && wide
      ? "collector"
      : canReadDashboards
        ? "overview"
        : "resources";
  const [tab, setTab] = useResourceDashboardTab(
    id,
    search.tab,
    initial,
    canReadDashboards,
    allowed,
  );
  if (access.isPending || (wide && cluster.isLoading))
    return (
      <PageShell title={t("shell.nav.clusterDetail")}>
        <Spinner label={t("common.loading")} />
      </PageShell>
    );
  if (!access.data || access.error || (wide && !cluster.data))
    return (
      <PageShell title={t("shell.nav.clusterDetail")}>
        <EmptyState
          description=""
          kind="error"
          title={t("kubernetes.loadFailed")}
        />
      </PageShell>
    );
  const name = cluster.data?.name ?? access.data.cluster_name;
  return (
    <PageShell
      breadcrumbs={[
        { label: t("shell.nav.kubernetes"), href: "/kubernetes" },
        { label: name },
      ]}
      description={cluster.data?.api_server}
      title={
        <span className="argus-k8s-cluster-card__head">
          {name}
          {cluster.data ? (
            <>
              <Badge tone="accent">
                {t("kubernetes.environment." + cluster.data.environment)}
              </Badge>
              <StatusBadge
                tone={connectionStatusTone(cluster.data.connection_status)}
              >
                {t("kubernetes.status." + cluster.data.connection_status)}
              </StatusBadge>
              {cluster.data.kubernetes_version && (
                <Badge>{cluster.data.kubernetes_version}</Badge>
              )}
            </>
          ) : (
            <Badge>{t("kubernetesWorkbench.namespaceOnly")}</Badge>
          )}
        </span>
      }
      actions={
        wide ? (
          <ResourceObserveLink type="kubernetes_cluster" id={id} />
        ) : undefined
      }
    >
      <Tabs
        value={tab}
        onValueChange={(value) => {
          setTab(value);
          location.update(
            { tab: value as NonNullable<typeof search.tab> },
            false,
          );
        }}
      >
        <TabsList>
          {canReadDashboards && (
            <TabsTrigger value="overview">
              {t("kubernetesResources.overviewTab")}
            </TabsTrigger>
          )}
          <TabsTrigger value="resources">
            {t("kubernetes.detail.resourcesTab")}
          </TabsTrigger>
          {wide && canReadDashboards && (
            <TabsTrigger value="dashboards">
              {t("dashboardLinks.resourceTab")}
            </TabsTrigger>
          )}
          {wide && (
            <TabsTrigger value="collector">
              {t("kubernetes.detail.collectorTab")}
            </TabsTrigger>
          )}
          <TabsTrigger value="activity">{t("operations.title")}</TabsTrigger>
        </TabsList>
        {canReadDashboards && (
          <TabsContent value="overview">
            <KubernetesClusterOverview
              clusterId={id}
              access={access.data}
              visible={tab === "overview"}
            />
          </TabsContent>
        )}
        <TabsContent value="resources">
          <KubernetesWorkbench key={id} clusterId={id} access={access.data} />
        </TabsContent>
        {wide && canReadDashboards && (
          <TabsContent value="dashboards">
            <ResourceDashboardLinks type="kubernetes_cluster" id={id} />
          </TabsContent>
        )}
        {wide && cluster.data && (
          <TabsContent value="collector">
            <div id="otlp-collector">
              {collector.isLoading ? (
                <Spinner label={t("common.loading")} />
              ) : collector.data ? (
                <CollectorStatusPanel
                  cluster={cluster.data}
                  collector={collector.data}
                />
              ) : (
                <CollectorWizard cluster={cluster.data} />
              )}
            </div>
          </TabsContent>
        )}
        <TabsContent value="activity">
          {wide ? (
            <ResourceExecutionActivity resourceId={id} resourceName={name} />
          ) : (
            <KubernetesOperationsDialog
              clusterId={id}
              embedded
              onClose={() => location.update({ tab: "resources" })}
            />
          )}
        </TabsContent>
      </Tabs>
    </PageShell>
  );
}
