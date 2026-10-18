import type { ArgusApiClient } from "../client";
import type { MockContext } from "./context";

export const activeRunStatuses = new Set([
  "pending",
  "running",
  "waiting_input",
  "waiting_approval",
  "waiting_system",
]);

export function createRunsDomain(ctx: MockContext): ArgusApiClient["runs"] {
  const owned = (id: string) => {
    const run = ctx.mustFind(
      ctx.db.conversationRuns ?? [],
      (r) => r.run_id === id,
      "run",
    );
    ctx.mustFind(
      ctx.db.conversations,
      (c) =>
        c.id === run.conversation_id &&
        c.enterpriseId === ctx.enterpriseId() &&
        c.createdBy === ctx.actor().id,
      "conversation",
    );
    return run;
  };
  return {
    async get(id) {
      return structuredClone(owned(id));
    },
    async cancel(id) {
      const run = owned(id);
      if (!activeRunStatuses.has(run.status))
        throw new Error("RUN_STATE_CONFLICT");
      run.status = "cancelled";
      run.stop_reason = "user_cancelled";
      run.updated_at = ctx.nowIso();
      run.version++;
      for (const item of ctx.db.interactions ?? []) {
        if (item.run_id === id && item.status === "pending") {
          item.status = "cancelled";
          item.version++;
        }
        if (item.implementation_run_id === id)
          item.implementation_status = "cancelled";
      }
      ctx.save();
      return structuredClone(run);
    },
    async compact() {
      throw new Error("mock run compaction is unavailable");
    },
  };
}
