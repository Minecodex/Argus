import { useQuery } from "@tanstack/react-query";
import {
  useApi,
  type PendingActionPublic,
  type AlertPolicySpec,
} from "@argus/api-client";
import { AlertActionPreview } from "./action-preview";

// Resolve display labels through the existing authorized read APIs. The
// immutable action still carries the exact IDs used by confirmation.
export function ResolvedAlertActionPreview({
  action,
}: {
  action: PendingActionPublic;
}) {
  const api = useApi();
  const enabled = action.action_type.startsWith("alert.");
  const preview = (action.preview ?? {}) as Record<string, unknown>;
  const spec = preview["spec"] as Partial<AlertPolicySpec> | undefined;
  const hasResources =
    Array.isArray(preview["resource_ids"]) &&
    preview["resource_ids"].length > 0;
  const channels = useQuery({
    queryKey: ["alerts", "configurations", "channel"],
    queryFn: () => api.alerts.configurations("channel"),
    enabled: enabled && !!spec?.channel_ids?.length,
  });
  const hosts = useQuery({
    queryKey: ["hosts", "alert-labels"],
    queryFn: () => api.hosts.list(),
    enabled: enabled && hasResources,
  });
  const clusters = useQuery({
    queryKey: ["clusters", "alert-labels"],
    queryFn: () => api.kubernetes.listClusters(),
    enabled: enabled && hasResources,
  });
  return (
    <AlertActionPreview
      action={action}
      layout={enabled ? "workspace" : "card"}
      references={{
        resources: Object.fromEntries(
          [...(hosts.data?.items ?? []), ...(clusters.data?.items ?? [])].map(
            (resource) => [resource.id, resource.name],
          ),
        ),
        channels: Object.fromEntries(
          (channels.data ?? []).map((channel) => [channel.id, channel.name]),
        ),
      }}
    />
  );
}
