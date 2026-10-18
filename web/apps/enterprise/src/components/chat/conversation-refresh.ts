import type { QueryClient } from "@tanstack/react-query";
import type { ConversationEvent } from "@argus/api-client/contracts";

/** Bound replay work and invalidate only the presentation named by each event. */
export function createConversationRefresh(
  cache: Pick<QueryClient, "invalidateQueries">,
  conversation: string,
) {
  const seen = new Set<string>(),
    calls = new Set<string>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let disposed = false;
  let interactions = false,
    settings = false;
  return {
    event(event: ConversationEvent) {
      if (
        disposed ||
        event.conversation_id !== conversation ||
        seen.has(event.event_id)
      )
        return;
      seen.add(event.event_id);
      if (
        [
          "interaction_requested",
          "interaction_resolved",
          "run_state_changed",
          "pending_action_created",
          "user_message",
        ].includes(event.event_type)
      )
        interactions = true;
      if (
        ["conversation_settings_changed", "interaction_resolved"].includes(
          event.event_type,
        )
      )
        settings = true;
      if (seen.size > 512) seen.delete(seen.values().next().value!);
      if (event.event_type === "tool_presentation") {
        const payload = event.payload;
        const call =
          payload && typeof payload === "object" && !Array.isArray(payload)
            ? (payload as Record<string, unknown>).tool_call_id
            : undefined;
        if (typeof call === "string" && call.length > 0 && call.length <= 128)
          calls.add(call);
      }
      if (timer !== undefined) return;
      timer = setTimeout(() => {
        timer = undefined;
        void cache.invalidateQueries({
          queryKey: ["conversations", conversation, "events"],
          exact: true,
        });
        for (const call of calls)
          void cache.invalidateQueries({
            queryKey: ["tool-presentation", conversation, call],
            exact: true,
          });
        calls.clear();
        if (interactions)
          void cache.invalidateQueries({
            queryKey: ["conversation-interactions", conversation],
          });
        if (settings)
          void cache.invalidateQueries({
            queryKey: ["conversations", "detail", conversation],
          });
        interactions = false;
        settings = false;
      }, 50);
    },
    dispose() {
      disposed = true;
      clearTimeout(timer);
      calls.clear();
      seen.clear();
    },
  };
}
