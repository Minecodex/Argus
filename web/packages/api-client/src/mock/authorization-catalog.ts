import type { BaseContext } from "./context";
import type {
  DataAuthorizationResource,
  DataAuthorizationResourceType,
} from "../types";
import { ApiError } from "../transport/errors";

type Resource = {
  id: string;
  name: string;
  environment?: string;
  parent_id?: string;
  parent_name?: string;
  source_uid?: string;
};
export function authorizationCatalog(
  ctx: BaseContext,
  type: DataAuthorizationResourceType,
): Resource[] {
  const id = ctx.enterpriseId(),
    db = ctx.db;
  switch (type) {
    case "host":
      return db.hosts.filter((v) => v.enterpriseId === id);
    case "kubernetes_cluster":
      return db.clusters.filter((v) => v.enterpriseId === id);
    case "dashboard":
      return (db.dashboards?.items ?? [])
        .filter((v) => v.enterprise === id)
        .map((v) => v.value);
    case "application":
      return (db.monitoring?.[id]?.applications ?? []).filter(
        (v) => v.status === "active",
      );
    case "monitoring_target":
      return (db.monitoring?.[id]?.targets ?? []).filter(
        (v) => v.status === "active",
      );
    case "kubernetes_namespace":
      return db.clusters
        .filter((v) => v.enterpriseId === id)
        .flatMap((cluster) =>
          (db.kubernetesInventory?.[cluster.id]?.namespaces ?? []).map(
            (ns) => ({
              id: ns.id,
              name: ns.name,
              parent_id: cluster.id,
              parent_name: cluster.name,
              source_uid: ns.uid,
              environment: cluster.environment,
            }),
          ),
        );
  }
}
const cursorMaps = new WeakMap<
  object,
  Map<string, { binding: string; offset: number; expires: number }>
>();
export function authorizationPage(
  ctx: BaseContext,
  rows: DataAuthorizationResource[],
  binding: unknown,
  cursor: string | undefined,
  limit: number | undefined,
  search: string,
) {
  const query = search.trim().toLowerCase(),
    catalog = rows
      .filter(
        (row) =>
          !query ||
          row.name.toLowerCase().includes(query) ||
          row.parent_name?.toLowerCase().includes(query) ||
          row.source_uid === search,
      )
      .sort((a, b) => a.resource_id.localeCompare(b.resource_id));
  let map = cursorMaps.get(ctx);
  if (!map) {
    map = new Map();
    cursorMaps.set(ctx, map);
  }
  const signature = JSON.stringify([
    ctx.actor().id,
    ctx.enterpriseId(),
    binding,
    query,
  ]);
  let offset = 0;
  if (cursor) {
    const entry = map.get(cursor);
    if (!entry || entry.binding !== signature || entry.expires < Date.now())
      throw new ApiError(
        {
          code: "CURSOR_INVALID",
          message_key: "errors.pagination.invalid",
          request_id: crypto.randomUUID(),
          retryable: false,
        },
        409,
      );
    offset = entry.offset;
  }
  const size = Math.min(Math.max(limit ?? 50, 1), 200),
    items = catalog.slice(offset, offset + size),
    has_more = offset + items.length < catalog.length;
  let next_cursor: string | null = null;
  if (has_more) {
    next_cursor = crypto.randomUUID();
    map.set(next_cursor, {
      binding: signature,
      offset: offset + items.length,
      expires: Date.now() + 600000,
    });
    if (map.size > 1000) map.delete(map.keys().next().value!);
  }
  return { items, page: { has_more, next_cursor } };
}
