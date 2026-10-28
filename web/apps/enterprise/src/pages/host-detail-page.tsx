import { HostOverview } from "../components/hosts/host-overview";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { useApi } from "@argus/api-client";
import {
  EmptyState,
  PageShell,
  Spinner,
  StatusBadge,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import "../styles/hosts.css";
import { ComponentsTab } from "../components/hosts/components-tab";
import { ResourceExecutionActivity } from "../components/operations/execution-result";
import { RealTerminalTab } from "../components/hosts/real-terminal-tab";
import {ResourceObserveLink} from "../components/observe/resource-link";
import { ResourceDashboardLinks } from "../components/dashboards/resource-dashboard-links";
import { useResourceDashboardTab } from "../components/dashboards/use-resource-dashboard-tab";
import { usePermission } from "../lib/permissions";
import {
  collectorTone,
  collectorStatusOf,
  hostConnectionPresentation,
} from "../components/hosts/host-utils";

const realMode = import.meta.env.VITE_API_MODE === "real";

/** 主机详情：资源概览、终端、采集与当前操作进度。历史统一关联企业审计。 */
export function HostDetailPage() {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();
  const { hostId } = useParams({ strict: false });
  const search = useSearch({ from: "/authed/admin/hosts/$hostId" });
  const canReadDashboards = usePermission("telemetry.dashboard.read");

  const hostQuery = useQuery({
    queryKey: ["hosts", "detail", hostId],
    queryFn: () => api.hosts.get(hostId ?? ""),
    enabled: Boolean(hostId),
    retry: false,
    refetchInterval: 30_000,
  });
  const scopesQuery = useQuery({
    queryKey: ["bastion-scopes"],
    queryFn: () => api.connectors.listBastionScopes(),
  });

  const host = hostQuery.data;
  const scopes = useMemo(
    () => scopesQuery.data?.items ?? [],
    [scopesQuery.data],
  );
  const initialTab =
    typeof window !== "undefined" && window.location.hash === "#otlp-collector"
      ? "components"
      : "overview";
  const [tab, setTab] = useResourceDashboardTab(
    hostId ?? "",
    initialTab === "components" ? undefined : search.tab,
    initialTab,
    canReadDashboards,
  );

  const invalidateAll = () => {
    void queryClient.invalidateQueries({ queryKey: ["hosts"] });
    void queryClient.invalidateQueries({ queryKey: ["bastion-scopes"] });
    void queryClient.invalidateQueries({ queryKey: ["connectors"] });
    void queryClient.invalidateQueries({ queryKey: ["host-collector"] });
    void queryClient.invalidateQueries({ queryKey: ["telemetry"] });
    void queryClient.invalidateQueries({
      queryKey: ["remote-access", "sessions"],
    });
    void queryClient.invalidateQueries({ queryKey: ["executions"] });
    void queryClient.invalidateQueries({ queryKey: ["audit"] });
  };

  return (
    <PageShell
      className="argus-host-detail-page"
      actions={host?<ResourceObserveLink type="host" id={host.id}/>:undefined}
      breadcrumbs={[
        { label: t("hosts.detail.backToList"), href: "/hosts" },
        { label: host?.name ?? hostId ?? "" },
      ]}
      title={
        host ? (
          <>
            {host.name}{" "}
            <StatusBadge
              pulse={hostConnectionPresentation(host).pulse}
              tone={hostConnectionPresentation(host).tone}
            >
              {t(hostConnectionPresentation(host).key)}
            </StatusBadge>{" "}
            {!realMode && (
              <StatusBadge tone={collectorTone(collectorStatusOf(host))}>
                {t(`hosts.collectorStatus.${collectorStatusOf(host)}`)}
              </StatusBadge>
            )}
          </>
        ) : (
          (hostId ?? "")
        )
      }
    >
      {hostQuery.isLoading && <Spinner />}
      {hostQuery.isError && (
        <EmptyState
          description=""
          kind="error"
          title={t("hosts.detail.notFound")}
        />
      )}
      {host && (
        <>
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="overview">
                {t("hosts.detail.tabOverview")}
              </TabsTrigger>
              {canReadDashboards && (
                <TabsTrigger value="dashboards">
                  {t("dashboardLinks.resourceTab")}
                </TabsTrigger>
              )}
              <TabsTrigger value="terminal">
                {t("hosts.detail.tabTerminal")}
              </TabsTrigger>
              <TabsTrigger value="components">
                {t("hosts.detail.tabComponents")}
              </TabsTrigger>
              <TabsTrigger value="activity">
                {t("operations.title")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="overview">
              <HostOverview host={host} />
            </TabsContent>
            {canReadDashboards && (
              <TabsContent value="dashboards">
                <ResourceDashboardLinks type="host" id={host.id} />
              </TabsContent>
            )}
            <TabsContent value="terminal">
              {realMode ? (
                <RealTerminalTab host={host} />
              ) : (
                <EmptyState
                  description={t("hosts.terminal.realOnlyDesc")}
                  title={t("hosts.terminal.realOnly")}
                />
              )}
            </TabsContent>
            <TabsContent value="components">
              <div id="otlp-collector">
                <ComponentsTab
                  host={host}
                  onChanged={invalidateAll}
                  scopes={scopes}
                />
              </div>
            </TabsContent>
            <TabsContent value="activity">
              <ResourceExecutionActivity
                resourceId={host.id}
                resourceName={host.name}
              />
            </TabsContent>
          </Tabs>
        </>
      )}
    </PageShell>
  );
}
