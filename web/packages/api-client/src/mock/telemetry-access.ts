import type { MockContext } from "./context";
import { resolvePermissions } from "./permissions";
import { dashboardObjectGrant } from "./dashboard-state";
import { ApiError } from "../transport/errors";

export function requireTelemetryResources(ctx: MockContext, ids: string[]) {
  const deny = (code: string): never => {
    throw new ApiError(
      {
        code,
        message_key: "errors.telemetry.query_scope_denied",
        request_id: crypto.randomUUID(),
        retryable: false,
      },
      403,
    );
  };
  if (!ids.length || ids.length > 1000) deny("QUERY_SCOPE_DENIED");
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  const allows = (name: string) =>
    permissions.has("*") || permissions.has(name);
  for (const id of ids) {
    const host = ctx.db.hosts.find(
      (h) => h.id === id && h.enterpriseId === ctx.enterpriseId(),
    );
    const cluster = ctx.db.clusters.find(
      (c) => c.id === id && c.enterpriseId === ctx.enterpriseId(),
    );
    const kind = host ? "host" : cluster ? "kubernetes_cluster" : "";
    if (
      !kind ||
      !allows(host ? "host.read" : "kubernetes.read") ||
      !dashboardObjectGrant(ctx, kind, id)
    )
      deny("QUERY_SCOPE_DENIED");
  }
}
