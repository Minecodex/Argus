import { useQuery } from "@tanstack/react-query";
import { useApi } from "@argus/api-client";
import { useMyPermissions } from "../../lib/permissions";
import type { ResourceMention } from "./use-resource-mentions";

/** Control-plane mentions share current authorization checks with scope mentions. */
export function useControlResourceMentions(enabled: boolean) {
  const api = useApi(),
    permissions = useMyPermissions();
  const mayRead = (permission: string) =>
    permissions.has("*") || permissions.has(permission);
  const canReadConnector = mayRead("connector.read"),
    canReadDashboard = mayRead("telemetry.dashboard.read");
  const connectors = useQuery({
    queryKey: ["connectors", "picker"],
    queryFn: () => api.connectors.list(),
    enabled: canReadConnector && enabled,
    retry: false,
    staleTime: 0,
  });
  const dashboards = useQuery({
    queryKey: ["dashboards", "chat-picker"],
    queryFn: () => api.dashboards.list(),
    enabled: canReadDashboard && enabled,
    retry: false,
    staleTime: 0,
  });
  const freshDashboard =
    enabled &&
    canReadDashboard &&
    !dashboards.isFetching &&
    !dashboards.isError;
  const freshConnector =
    enabled &&
    canReadConnector &&
    !connectors.isFetching &&
    !connectors.isError;
  const items: ResourceMention[] = [
    ...(freshDashboard ? (dashboards.data ?? []) : [])
      .filter(
        (item) =>
          item.lifecycle === "active" &&
          item.managed_template_key !== "argus.model-governance",
      )
      .map((item) => ({
        kind: "dashboard" as const,
        id: item.id,
        label: item.name,
      })),
    ...(freshConnector ? (connectors.data?.items ?? []) : []).map((item) => ({
      kind: "connector" as const,
      id: item.id,
      label: item.name,
    })),
  ];
  return {
    items,
    connectors,
    dashboards,
    isFetching:
      (canReadConnector && connectors.isFetching) ||
      (canReadDashboard && dashboards.isFetching),
  };
}
