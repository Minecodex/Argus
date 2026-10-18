import type { RemoteExecutionDomains } from "../remote-execution";
import type { MockContext } from "./context";
export function createRemoteExecutionDomain(
  ctx: MockContext,
): RemoteExecutionDomains["remoteExecutions"] {
  const find = (conversation: string, id: string) => {
    ctx.mustFind(
      ctx.db.conversations,
      (item) =>
        item.id === conversation &&
        item.enterpriseId === ctx.enterpriseId() &&
        item.createdBy === ctx.actor().id,
      "conversation",
    );
    return ctx.mustFind(
      ctx.db.remoteExecutions ?? [],
      (item) => item.id === id && item.spec.conversation_id === conversation,
      "remote execution",
    );
  };
  return {
    inputUrl(conversation, id, fileId) {
      const view = find(conversation, id);
      const file = view.spec.inputs.find((f) => f.id === fileId);
      return file?.content_preview !== undefined
        ? `data:application/octet-stream;charset=utf-8,${encodeURIComponent(file.content_preview)}`
        : undefined;
    },
    async list(conversation) {
      await ctx.pause();
      return structuredClone(
        (ctx.db.remoteExecutions ?? []).filter(
          (item) =>
            item.spec.conversation_id === conversation &&
            ctx.db.conversations.some(
              (c) =>
                c.id === conversation &&
                c.enterpriseId === ctx.enterpriseId() &&
                c.createdBy === ctx.actor().id,
            ),
        ),
      );
    },
    async get(conversation, id) {
      await ctx.pause();
      return structuredClone(find(conversation, id));
    },
    async stop(conversation, id) {
      const item = find(conversation, id);
      item.stop_requested = true;
      item.stop_status = "requested";
      item.status = ["ready", "running"].includes(item.status)
        ? "stopping"
        : item.status;
      ctx.save();
      return structuredClone(item);
    },
    async feedback(conversation, id) {
      const item = find(conversation, id);
      if (item.status !== "awaiting_confirmation")
        throw new Error("REMOTE_PLAN_NOT_REVISABLE");
      item.status = "superseded";
      ctx.save();
      return structuredClone(item);
    },
  };
}
