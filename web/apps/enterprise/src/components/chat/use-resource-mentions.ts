import { useInfiniteQuery } from "@tanstack/react-query";
import { useApi, type ObservationResourceRef } from "@argus/api-client";
import { useMyPermissions } from "../../lib/permissions";
import { observationReferenceTypes } from "./observation-reference";

export type ResourceMention = {
  kind: ObservationResourceRef["type"] | "connector" | "dashboard";
  id: string;
  label: string;
  context?: string;
};

/** Pages come from the authoritative scope directory, not a first-page client filter. */
export function useResourceMentions(search: string, enabled: boolean) {
  const api = useApi();
  const permissions = useMyPermissions();
  const canRead = (kind: string) =>
    permissions.has("*") ||
    permissions.has(
      kind.startsWith("kubernetes_")
        ? "kubernetes.read"
        : kind === "monitoring_target"
          ? "monitoring.read"
          : `${kind}.read`,
    );
  const selected = observationReferenceTypes.filter(canRead);
  const scopeKey = selected.join(",");
  const query = useInfiniteQuery({
    queryKey: ["chat-resource-mentions", search, scopeKey],
    enabled: enabled && selected.length > 0,
    initialPageParam: {} as Partial<
      Record<ObservationResourceRef["type"], string | null>
    >,
    queryFn: async ({ pageParam, signal }) => {
      const result = await Promise.all(
        selected
          .filter((kind) => pageParam[kind] !== null)
          .map(async (kind) => {
            const page = await api.telemetry.resources(
              { kind, search, cursor: pageParam[kind] ?? undefined, limit: 20 },
              signal,
            );
            return { kind, page };
          }),
      );
      const cursors = { ...pageParam };
      const items: ResourceMention[] = [];
      for (const { kind, page } of result) {
        cursors[kind] = page.next_cursor || null;
        for (const row of page.items)
          items.push({
            kind: row.type,
            id: row.id,
            label: row.name,
            context: [row.environment, row.parent_name]
              .filter(Boolean)
              .join(" · "),
          });
      }
      return { items, cursors };
    },
    getNextPageParam: (last) =>
      Object.values(last.cursors).some(Boolean) ? last.cursors : undefined,
  });
  const items = new Map<string, ResourceMention>();
  for (const page of query.data?.pages ?? [])
    for (const item of page.items) items.set(`${item.kind}:${item.id}`, item);
  // Opening/refetching requires fresh authorization. Next-page loading keeps
  // the rows already confirmed during this open directory session.
  const checkingAccess = query.isFetching && !query.isFetchingNextPage;
  return {
    ...query,
    items: query.isError || checkingAccess ? [] : [...items.values()],
  };
}
