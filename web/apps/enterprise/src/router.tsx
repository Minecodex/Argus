import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { useEnterpriseAuthStore } from "@argus/auth";
const AdminShell = lazyRouteComponent(() => import("./components/admin-shell"), "AdminShell");
const ChatShell = lazyRouteComponent(() => import("./components/chat-shell"), "ChatShell");

const ObservePage=lazyRouteComponent(()=>import("./pages/observe-page"),"ObservePage");
const ApplicationsPage = lazyRouteComponent(
  () => import("./pages/applications-page"),
  "ApplicationsPage",
);
const MonitoringPage = lazyRouteComponent(
  () => import("./pages/monitoring-page"),
  "MonitoringPage",
);
const AlertsPage = lazyRouteComponent(
  () => import("./pages/alerts-page"),
  "AlertsPage",
);
const DashboardsPage = lazyRouteComponent(
  () => import("./pages/dashboards-page"),
  "DashboardsPage",
);
const DashboardViewPage = lazyRouteComponent(
  () => import("./pages/dashboard-view-page"),
  "DashboardViewPage",
);
const DashboardDraftWorkspace = lazyRouteComponent(
  () => import("./components/dashboards/draft-workspace"),
  "DashboardDraftWorkspace",
);
const DashboardEditorPage = lazyRouteComponent(
  () => import("./pages/dashboard-editor-page"),
  "DashboardEditorPage",
);
const DashboardPanelEditorPage = lazyRouteComponent(
  () => import("./pages/dashboard-panel-editor-page"),
  "DashboardPanelEditorPage",
);
const LoginPage = lazyRouteComponent(
  () => import("./pages/login-page"),
  "LoginPage",
);
const AccountPage = lazyRouteComponent(
  () => import("./pages/account-page"),
  "AccountPage",
);
const ConversationPage = lazyRouteComponent(
  () => import("./pages/conversation-page"),
  "ConversationPage",
);
const HostsPage = lazyRouteComponent(
  () => import("./pages/hosts-page"),
  "HostsPage",
);
const HostDetailPage = lazyRouteComponent(
  () => import("./pages/host-detail-page"),
  "HostDetailPage",
);
const KubernetesPage = lazyRouteComponent(
  () => import("./pages/kubernetes-page"),
  "KubernetesPage",
);
const KubernetesClusterPage = lazyRouteComponent(
  () => import("./pages/kubernetes-cluster-page"),
  "KubernetesClusterPage",
);
const ApprovalsPage = lazyRouteComponent(
  () => import("./pages/approvals-page"),
  "ApprovalsPage",
);
const RemoteSessionsPage = lazyRouteComponent(
  () => import("./pages/remote-sessions-page"),
  "RemoteSessionsPage",
);
const SettingsOrgPage = lazyRouteComponent(
  () => import("./pages/settings-org-page"),
  "SettingsOrgPage",
);
const SettingsAiopsPage = lazyRouteComponent(
  () => import("./pages/settings-aiops-page"),
  "SettingsAiopsPage",
);
const SettingsSecretsPage = lazyRouteComponent(
  () => import("./pages/settings-secrets-page"),
  "SettingsSecretsPage",
);
const SettingsAuditPage = lazyRouteComponent(
  () => import("./pages/settings-audit-page"),
  "SettingsAuditPage",
);
const DemoPage = lazyRouteComponent(
  () => import("./pages/demo-page"),
  "DemoPage",
);

const rootRoute = createRootRoute({ component: () => <Outlet /> });

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  validateSearch: (search: Record<string, unknown>) => ({
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  }),
  component: LoginPage,
});

/**
 * 受保护区域：未登录访问任何子路由都会在 beforeLoad 阶段
 * 重定向到 /login，并带上 redirect 回跳参数。
 */
const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "authed",
  beforeLoad: ({ location }) => {
    const auth = useEnterpriseAuthStore.getState();
    if (auth.status === "unknown" || auth.status === "checking") return;
    const session = auth.session;
    if (!session || session.session.audience !== "enterprise") {
      throw redirect({
        to: "/login",
        search: { redirect: location.href },
      });
    }
  },
  component: () => <Outlet />,
});

// Chatbox 布局：企业门户的默认模式
const chatRoute = createRoute({
  getParentRoute: () => authedRoute,
  id: "chat",
  component: ChatShell,
});
const indexRoute = createRoute({
  getParentRoute: () => chatRoute,
  path: "/",
  validateSearch: (search: Record<string, unknown>) => ({
    c: typeof search.c === "string" && search.c ? search.c : undefined,
  }),
  component: ConversationPage,
});

// 管理后台布局
const adminRoute = createRoute({
  getParentRoute: () => authedRoute,
  id: "admin",
  component: AdminShell,
});
const observeRoute=createRoute({getParentRoute:()=>adminRoute,path:"/observe",component:ObservePage,validateSearch:(search:Record<string,unknown>)=>({
 resource_type:(["host","kubernetes_cluster","kubernetes_namespace","application","monitoring_target"].includes(String(search.resource_type))?search.resource_type:undefined) as import("@argus/api-client").ObservationResourceRef["type"]|undefined,
 resource_id:typeof search.resource_id==="string"?search.resource_id:undefined,
 signal:(["metrics","logs","traces"].includes(String(search.signal))?search.signal:"metrics") as import("@argus/api-client").ObservationSignal,
})});
const applicationsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/applications",
  component: ApplicationsPage,
});
const applicationDetailRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/applications/$applicationId",
  component: ApplicationsPage,
});
const alertsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/alerts",
  component: AlertsPage,
});
const dashboardsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/dashboards",
  component: DashboardsPage,
});
const dashboardViewRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/dashboards/$dashboardId",
  component: DashboardViewPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    resource?: string;
    model?: string;
    month?: string;
    resource_type?:
      "host" | "kubernetes_cluster" | "application" | "monitoring_target";
  } => ({
    resource: typeof search.resource === "string" ? search.resource : undefined,
    model: typeof search.model === "string" ? search.model : undefined,
    month: typeof search.month === "string" ? search.month : undefined,
    resource_type:
      search.resource_type === "host" ||
      search.resource_type === "kubernetes_cluster" ||
      search.resource_type === "application" ||
      search.resource_type === "monitoring_target"
        ? search.resource_type
        : undefined,
  }),
});
const monitoringRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/monitoring",
  component: MonitoringPage,
});
const dashboardEditorRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/dashboard-drafts/$draftId",
  component: DashboardDraftWorkspace,
});
const dashboardPanelEditorRoute = createRoute({
  getParentRoute: () => dashboardEditorRoute,
  path: "/panels/$panelId",
  component: DashboardPanelEditorPage,
});
const dashboardDraftIndexRoute = createRoute({
  getParentRoute: () => dashboardEditorRoute,
  path: "/",
  component: DashboardEditorPage,
});
const hostsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/hosts",
  component: HostsPage,
});
const hostDetailRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/hosts/$hostId",
  component: HostDetailPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): { tab?: "dashboards" | "activity" } => ({
    tab:
      search.tab === "dashboards" || search.tab === "activity"
        ? search.tab
        : undefined,
  }),
});
const kubernetesRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/kubernetes",
  component: KubernetesPage,
});
const kubernetesClusterRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/kubernetes/$clusterId",
  component: KubernetesClusterPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): { tab?: "dashboards" | "activity" } => ({
    tab:
      search.tab === "dashboards" || search.tab === "activity"
        ? search.tab
        : undefined,
  }),
});
const approvalsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/approvals",
  validateSearch: (search: Record<string, unknown>) => ({
    approval: search.approval === "remote" ? "remote" : "operation",
    scope: ["mine", "created", "done"].includes(String(search.scope))
      ? (String(search.scope) as "mine" | "created" | "done")
      : "mine",
  }),
  component: ApprovalsPage,
});
const remoteSessionsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/remote-sessions",
  component: RemoteSessionsPage,
});
const accountRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/account",
  component: AccountPage,
});
const settingsOrgRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/org",
  component: SettingsOrgPage,
});
const settingsAiopsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/aiops",
  component: SettingsAiopsPage,
  validateSearch: (search: Record<string, unknown>): { tab?: "models" | "mcp" } => ({
    tab: search.tab === "models" || search.tab === "mcp" ? search.tab : undefined,
  }),
});
const settingsAiRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/ai",
  beforeLoad: () => {
    throw redirect({ to: "/settings/aiops", search: { tab: "models" }, replace: true });
  },
});
const settingsMCPRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/mcp",
  beforeLoad: () => {
    throw redirect({ to: "/settings/aiops", search: { tab: "mcp" }, replace: true });
  },
});
const settingsSecretsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/secrets",
  component: SettingsSecretsPage,
});
const settingsAuditRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/settings/audit",
  component: SettingsAuditPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): { operation_ref?: string } => ({
    operation_ref:
      typeof search.operation_ref === "string" &&
      search.operation_ref.length <= 256
        ? search.operation_ref
        : undefined,
  }),
});

// 组件展厅仅开发环境注册，不进入任何菜单
const demoRoute = import.meta.env.DEV
  ? createRoute({
      getParentRoute: () => adminRoute,
      path: "/demo",
      component: DemoPage,
    })
  : undefined;

const routeTree = rootRoute.addChildren([
  loginRoute,
  authedRoute.addChildren([
    chatRoute.addChildren([indexRoute]),
    adminRoute.addChildren([
      dashboardsRoute,
      observeRoute,
      alertsRoute,
      applicationsRoute,
      monitoringRoute,
      applicationDetailRoute,
      dashboardViewRoute,
      dashboardEditorRoute.addChildren([
        dashboardDraftIndexRoute,
        dashboardPanelEditorRoute,
      ]),
      hostsRoute,
      hostDetailRoute,
      kubernetesRoute,
      kubernetesClusterRoute,
      approvalsRoute,
      remoteSessionsRoute,
      accountRoute,
      settingsOrgRoute,
      settingsAiopsRoute,
      settingsAiRoute,
      settingsMCPRoute,
      settingsSecretsRoute,
      settingsAuditRoute,
      ...(demoRoute ? [demoRoute] : []),
    ]),
  ]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
