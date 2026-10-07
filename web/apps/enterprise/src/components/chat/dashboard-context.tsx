import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { LayoutDashboard, X } from "lucide-react";
import { useApi, type DashboardChatSelection } from "@argus/api-client";
import { Button } from "@argus/ui";
import { usePermission } from "../../lib/permissions";

export function DashboardMessageReference({
  selection,
}: {
  selection: DashboardChatSelection;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const canRead = usePermission("telemetry.dashboard.read");
  const items = useQuery({
    queryKey: ["dashboards", "chat-picker"],
    queryFn: () => api.dashboards.list(),
    enabled: canRead && selection.mode !== "none",
  });
  if (selection.mode === "none") return null;
  return (
    <div
      className="argus-chat-composer__chips"
      aria-label={t("chat.dashboard.context")}
    >
      <span className="argus-chat-chip">
        <LayoutDashboard size={12} />
        {t("chat.dashboard." + selection.mode)}
      </span>
      {selection.dashboard_ids.map((id) => (
        <span className="argus-chat-chip" key={id}>
          {items.data?.find((item) => item.id === id)?.name ??
            t("chat.dashboard.unavailable")}
        </span>
      ))}
    </div>
  );
}

export function useDashboardChatContext(conversationId?: string) {
  const api = useApi();
  const current = useQuery({
    queryKey: ["conversation-dashboard-context", conversationId],
    queryFn: () => api.conversations.dashboardContext(conversationId!),
    enabled: !!conversationId,
  });
  const [edited, setEdited] = useState<DashboardChatSelection | null>(null);
  const previous = useRef(conversationId);
  useEffect(() => {
    if (previous.current && previous.current !== conversationId)
      setEdited(null);
    previous.current = conversationId;
  }, [conversationId]);
  const selection: DashboardChatSelection = edited ?? {
    mode: current.data?.mode ?? "none",
    dashboard_ids: current.data?.dashboard_ids ?? [],
    expected_version: current.data?.version ?? 0,
  };
  return {
    selection,
    ready: !conversationId || !!current.data,
    failed: current.isError,
    change: (mode: DashboardChatSelection["mode"], ids: string[]) =>
      setEdited({ ...selection, mode, dashboard_ids: ids }),
    reload: async () => {
      if (!conversationId) {
        setEdited(null);
        return;
      }
      const result = await current.refetch();
      if (result.isSuccess) setEdited(null);
    },
    accepted: () => setEdited(null),
  };
}

export function DashboardChatContextBar({
  selection,
  items,
  disabled,
  failed,
  onChange,
  onReload,
}: {
  selection: DashboardChatSelection;
  items: Array<{ id: string; name: string }>;
  disabled: boolean;
  failed: boolean;
  onChange: (mode: DashboardChatSelection["mode"], ids: string[]) => void;
  onReload: () => void;
}) {
  const { t } = useTranslation();
  if (selection.mode === "none" && !failed) return null;
  return (
    <div
      className="argus-chat-composer__chips"
      aria-label={t("chat.dashboard.context")}
    >
      {selection.mode !== "none" && (
        <>
          <span className="argus-chat-chip">
            <LayoutDashboard size={12} />
            {t("chat.dashboard." + selection.mode)}
          </span>
          {selection.dashboard_ids.map((id) => (
            <span className="argus-chat-chip" key={id}>
              {items.find((item) => item.id === id)?.name ??
                t("chat.dashboard.unavailable")}
              <Button
                variant="ghost"
                type="button"
                isDisabled={disabled}
                aria-label={t("chat.dashboard.remove")}
                onPress={() =>
                  onChange(
                    selection.mode,
                    selection.dashboard_ids.filter((value) => value !== id),
                  )
                }
              >
                <X size={11} />
              </Button>
            </span>
          ))}
          {!selection.dashboard_ids.length && selection.mode === "analyze" && (
            <span>{t("chat.dashboard.select")}</span>
          )}
          <Button
            variant="ghost"
            size="sm"
            isDisabled={disabled}
            onPress={() => onChange("none", [])}
          >
            {t("chat.dashboard.exit")}
          </Button>
        </>
      )}
      {failed && <span role="alert">{t("chat.dashboard.failed")}</span>}
      <Button
        variant="ghost"
        size="sm"
        isDisabled={disabled}
        onPress={onReload}
      >
        {t("chat.dashboard.reload")}
      </Button>
    </div>
  );
}
