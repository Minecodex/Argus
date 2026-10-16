import { useQueries } from "@tanstack/react-query";
import { useApi } from "@argus/api-client";
import { useMyPermissions } from "../../lib/permissions";
import type { ChatMessage } from "./chat-view-model";

export type ConversationResource = {
  id: string;
  type: string;
  name?: string;
  href?: string;
  unavailable?: boolean;
};
const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const supported = new Set([
  "host",
  "kubernetes_cluster",
  "connector",
  "dashboard",
]);

/** Resolve actual business references, never treat a ToolCall/Template ID as a resource. */
export function useConversationResources(messages: ChatMessage[]) {
  const api = useApi();
  const permissions = useMyPermissions();
  const calls = [
    ...new Set(messages.flatMap((message) => message.presentations ?? [])),
  ];
  const conversation = messages[0]?.conversationId ?? "";
  const presentations = useQueries({
    queries: calls.map((id) => ({
      queryKey: ["tool-presentation", conversation, id],
      queryFn: () => api.presentations.get(conversation, id),
      retry: false,
    })),
  });
  const references = new Map<string, ConversationResource>();
  const add = (id: unknown, type: unknown) => {
    if (
      typeof id === "string" &&
      typeof type === "string" &&
      supported.has(type)
    )
      references.set(`${type}:${id}`, { id, type });
  };
  const candidates = new Map<string, string>();
  const dashboardScopes = new Map<string, string[]>();
  for (const message of messages) {
    for (const id of message.dashboardContext?.dashboard_ids ?? [])
      add(id, "dashboard");
    for (const call of message.toolCalls ?? []) {
      const output = record(call.output);
      for (const resource of Array.isArray(output.resources)
        ? output.resources
        : []) {
        const value = record(resource);
        if (typeof value.id === "string" && typeof value.type === "string")
          candidates.set(value.id, value.type);
      }
    }
  }
  for (const message of messages)
    for (const call of message.toolCalls ?? []) {
      const input = record(call.input);
      if (
        call.toolName === "telemetry.dashboard.query" &&
        typeof input.dashboard_id === "string" &&
        Array.isArray(input.resource_ids)
      )
        dashboardScopes.set(
          input.dashboard_id,
          input.resource_ids.filter(
            (id): id is string => typeof id === "string",
          ),
        );
      const changes = record(input.changes);
      const selected = record(changes.resources);
      for (const id of Array.isArray(selected.ids)
        ? selected.ids
        : Array.isArray(input.resource_ids)
          ? input.resource_ids
          : [])
        add(id, candidates.get(String(id)));
    }
  for (const query of presentations)
    if (!query.isError)
      for (const ref of query.data?.resource_refs ?? []) add(ref.id, ref.type);
  const values = [...references.values()];
  const mayRead = (type: string) =>
    permissions.has("*") ||
    permissions.has(
      type === "dashboard"
        ? "telemetry.dashboard.read"
        : type === "kubernetes_cluster"
          ? "kubernetes.read"
          : `${type}.read`,
    );
  const resolved = useQueries({
    queries: values.map((ref) => ({
      queryKey: [
        "conversation-resource-name",
        conversation,
        ref.type,
        ref.id,
        ref.type === "dashboard"
          ? (dashboardScopes.get(ref.id)?.join(",") ?? "")
          : "",
      ],
      enabled: mayRead(ref.type),
      retry: false,
      staleTime: 0,
      queryFn: async (): Promise<ConversationResource> => {
        if (ref.type === "host") {
          const value = await api.hosts.get(ref.id);
          return { ...ref, name: value.name, href: `/hosts/${ref.id}` };
        }
        if (ref.type === "kubernetes_cluster") {
          const value = await api.kubernetes.getCluster(ref.id);
          return { ...ref, name: value.name, href: `/kubernetes/${ref.id}` };
        }
        if (ref.type === "dashboard") {
          const value = await api.dashboards.get(ref.id);
          const scope = dashboardScopes.get(ref.id);
          return {
            ...ref,
            name: value.dashboard.name,
            href: `/dashboards/${ref.id}${scope?.length === 1 ? `?resource=${encodeURIComponent(scope[0]!)}` : ""}`,
          };
        }
        const value = await api.connectors.get(ref.id);
        const hosts = mayRead("host") ? await api.hosts.list() : undefined;
        const host = hosts?.items.find((item) => item.connector_id === ref.id);
        return {
          ...ref,
          name: value.name,
          href: host ? `/hosts/${host.id}` : undefined,
        };
      },
    })),
  });
  return values.map((ref, index) =>
    !mayRead(ref.type) || resolved[index]?.isError
      ? { ...ref, unavailable: true }
      : (resolved[index]?.data ?? ref),
  );
}
