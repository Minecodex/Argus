import type { InteractionDomains } from "../interactions";
import type { MockContext } from "./context";
import { activeRunStatuses } from "./runs";
export function createInteractionDomains(
  ctx: MockContext,
): InteractionDomains["interactions"] {
  const owned = (conversation: string) =>
    ctx.mustFind(
      ctx.db.conversations,
      (c) =>
        c.id === conversation &&
        c.enterpriseId === ctx.enterpriseId() &&
        c.createdBy === ctx.actor().id,
      "conversation",
    );
  const get = (conversation: string, id: string) => {
    owned(conversation);
    return ctx.mustFind(
      ctx.db.interactions ?? [],
      (x) => x.conversation_id === conversation && x.id === id,
      "interaction",
    );
  };
  return {
    async list(conversation) {
      owned(conversation);
      const active = ctx.db.conversationRuns?.find(
        (run) =>
          run.conversation_id === conversation &&
          activeRunStatuses.has(run.status),
      );
      return {
        ...(active
          ? { active_run_id: active.run_id, active_run_status: active.status }
          : {}),
        items: structuredClone(
          (ctx.db.interactions ?? []).filter(
            (x) =>
              x.conversation_id === conversation && x.status !== "superseded",
          ),
        ),
      };
    },
    async get(conversation, id) {
      return structuredClone(get(conversation, id));
    },
    async respond(conversation, id, response) {
      const value = get(conversation, id);
      if (
        value.status !== "pending" ||
        value.version !== response.version ||
        value.content_hash !== response.content_hash
      )
        throw new Error("RUN_STATE_CONFLICT");
      if (value.kind === "plan") {
        if (response.action === "confirm") {
          value.status = "confirmed";
          value.implementation_run_id = crypto.randomUUID();
          value.implementation_status = "pending";
          const c = owned(conversation);
          c.mode = "execute";
          c.version = (c.version ?? 1) + 1;
          ctx.db.conversationRuns ??= [];
          ctx.db.conversationRuns.push({
            run_id: value.implementation_run_id,
            conversation_id: conversation,
            enterprise_id: c.enterpriseId,
            model_id: c.selectedModelId,
            model_revision: 1,
            locale: "zh-CN",
            status: "pending",
            mode_snapshot: "execute",
            access_mode_snapshot: c.accessMode ?? "request_approval",
            approved_plan_id: value.id,
            authorization_version: 1,
            version: 1,
            created_at: ctx.nowIso(),
            updated_at: ctx.nowIso(),
          });
          ctx.db.messages.push({
            id: crypto.randomUUID(),
            conversationId: conversation,
            role: "user",
            content: "是的，执行此计划",
            createdAt: ctx.nowIso(),
          });
        } else if (response.action === "revise" && response.feedback?.trim()) {
          value.status = "superseded";
          ctx.db.messages.push({
            id: crypto.randomUUID(),
            conversationId: conversation,
            role: "user",
            content: response.feedback,
            createdAt: ctx.nowIso(),
          });
        } else throw new Error("TOOL_INPUT_INVALID");
      } else {
        if (!["answer", "skip"].includes(response.action))
          throw new Error("TOOL_INPUT_INVALID");
        if (
          response.action === "answer" &&
          value.content.questions?.some((q) => {
            const a = response.answers?.[q.id];
            return (
              !a ||
              (!a.skipped && !a.text?.trim() && !a.selected_option) ||
              (a.selected_option &&
                !q.options.some((o) => o.label === a.selected_option))
            );
          })
        )
          throw new Error("TOOL_INPUT_INVALID");
        value.status = response.action === "skip" ? "skipped" : "answered";
      }
      value.response = response;
      value.version += 1;
      value.updated_at = ctx.nowIso();
      ctx.save();
      return structuredClone(value);
    },
  };
}
