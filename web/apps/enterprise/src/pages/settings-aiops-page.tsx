import { Navigate, useNavigate, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  Alert,
  PageShell,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import { useAiopsAccess } from "../lib/aiops-access";
import { ModelSettingsPanel } from "./settings-ai-page";
import { MCPSettingsPanel } from "./settings-mcp-page";
import "../styles/settings.css";

export function SettingsAiopsPage() {
  const { t } = useTranslation();
  const { tab: requestedTab } = useSearch({
    from: "/authed/admin/settings/aiops",
  });
  const navigate = useNavigate({ from: "/settings/aiops" });
  const { canViewModels, canManageMCP, canAccessAiops } = useAiopsAccess();
  const tab =
    requestedTab === "mcp" && canManageMCP
      ? "mcp"
      : canViewModels
        ? "models"
        : "mcp";

  if (canAccessAiops && requestedTab && requestedTab !== tab) {
    return <Navigate to="/settings/aiops" search={{ tab }} replace />;
  }

  return (
    <PageShell
      title={t("aiopsSettings.title")}
      description={t("aiopsSettings.description")}
    >
      {!canAccessAiops ? (
        <Alert
          tone="danger"
          title={t("aiopsSettings.forbidden")}
          description={t("aiopsSettings.forbiddenDescription")}
        />
      ) : (
        <Tabs
          value={tab}
          onValueChange={(value) =>
            void navigate({
              search: { tab: value === "mcp" ? "mcp" : "models" },
            })
          }
        >
          <TabsList>
            {canViewModels && (
              <TabsTrigger value="models">
                {t("shell.nav.settingsAi")}
              </TabsTrigger>
            )}
            {canManageMCP && (
              <TabsTrigger value="mcp">{t("planv5.mcp.title")}</TabsTrigger>
            )}
          </TabsList>
          {canViewModels && (
            <TabsContent value="models">
              <ModelSettingsPanel />
            </TabsContent>
          )}
          {canManageMCP && (
            <TabsContent value="mcp">
              <MCPSettingsPanel />
            </TabsContent>
          )}
        </Tabs>
      )}
    </PageShell>
  );
}
