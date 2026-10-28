import type { ArgusApiClient } from "../client";
import type { MockContext } from "./context";

/** Enterprise audit trail queries. */
export function createAuditDomain(ctx: MockContext): ArgusApiClient["audit"] {
  const { db } = ctx;

  return {
    async list(filter, query) {
      await ctx.pause();
      let items = db.auditEvents
        .filter((entry) => entry.enterpriseId === ctx.enterpriseId())
        .map((entry) => {
          const execution = db.executions.find(
            (item) => item.execution_id === entry.resourceId,
          );
          const ref =
            entry.details?.action_ref ??
            (entry.resourceType === "pending_action"
              ? entry.resourceId
              : execution?.action_ref);
          const action = db.pendingActions.find(
            (item) => item.action_ref === ref,
          );
          if (!action) return entry;
          const current =
            execution ?? db.executions.find((item) => item.action_ref === ref);
          const target = current?.resource_ref;
          const resourceName =
            db.hosts.find((item) => item.id === target?.resource_id)?.name ??
            db.clusters.find((item) => item.id === target?.resource_id)?.name;
          return {
            ...entry,
            operation: {
              action_ref: action.action_ref,
              action_type: action.action_type,
              action_status: action.status,
              execution_id: current?.execution_id,
              execution_status: current?.status,
              error_code: current?.error_code,
              resource_type: target?.resource_type,
              resource_id: target?.resource_id,
              resource_name: resourceName,
              created_at: action.created_at,
              updated_at: current?.updated_at ?? action.updated_at,
            },
          };
        });
      if (filter?.action) {
        items = items.filter((entry) => entry.action === filter.action);
      }
      if (filter?.operationRef) {
        items = items.filter(
          (entry) =>
            entry.operation?.action_ref === filter.operationRef ||
            entry.details?.action_ref === filter.operationRef,
        );
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
