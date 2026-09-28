import type { ArgusApiClient } from "../client";
import type { MockContext } from "./context";

/** Enterprise audit trail queries. */
export function createAuditDomain(ctx: MockContext): ArgusApiClient["audit"] {
  const { db } = ctx;

  return {
    async list(filter, query) {
      await ctx.pause();
      let items = db.auditEvents.filter(
        (entry) => entry.enterpriseId === ctx.enterpriseId(),
      );
      if (filter?.action) {
        items = items.filter((entry) => entry.action === filter.action);
      }
      if (filter?.actorUserId) {
        items = items.filter(
          (entry) => entry.actorUserId === filter.actorUserId,
        );
      }
      if (filter?.resourceType) {
        items = items.filter(
          (entry) => entry.resourceType === filter.resourceType,
        );
      }
      if (filter?.resourceId)
        items = items.filter((entry) => entry.resourceId === filter.resourceId);
      if (filter?.from)
        items = items.filter((entry) => entry.createdAt >= filter.from!);
      if (filter?.to)
        items = items.filter((entry) => entry.createdAt < filter.to!);
      if (filter?.result) {
        items = items.filter((entry) => entry.result === filter.result);
      }
      if (filter?.query) {
        items = items.filter((entry) =>
          [
            entry.action,
            entry.actorUserId,
            entry.resourceType,
            entry.resourceId,
            entry.summary,
            JSON.stringify(entry.details ?? {}),
          ]
            .join(" ")
            .toLowerCase()
            .includes(filter.query!.trim().toLowerCase()),
        );
      }
      return ctx.paginate(items, query);
    },
  };
}
