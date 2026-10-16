import type { ChatMessage } from "./chat-view-model";
import { hasChatContent } from "./chat-content";

/** Join the visual result to its call without changing ledger events or their order. */
export function combineToolActivities(
  timeline: readonly ChatMessage[],
): ChatMessage[] {
  const key = (conversation: string, call: string) => `${conversation}:${call}`;
  const owners = new Map<string, number>();
  const result = timeline.map((message, index) => {
    if (message.kind !== "presentation")
      for (const call of message.toolCalls ?? [])
        owners.set(key(message.conversationId, call.callId), index);
    return {
      ...message,
      presentations: [...(message.presentations ?? [])],
      toolCalls: [...(message.toolCalls ?? [])],
      pendingActionRefs: [...(message.pendingActionRefs ?? [])],
    };
  });
  for (const [index, message] of result.entries()) {
    if (message.kind !== "presentation") continue;
    message.presentations = message.presentations.filter((id) => {
      const owner = owners.get(key(message.conversationId, id));
      // Keep a standalone result at its original position when no earlier call exists.
      if (owner === undefined || owner >= index) return true;
      const destination = result[owner]!;
      if (!destination.presentations.includes(id))
        destination.presentations.push(id);
      return false;
    });
    // The trace on a presentation event is metadata, not another tool invocation.
    message.toolCalls = message.toolCalls?.filter((call) =>
      message.presentations.includes(call.callId),
    );
  }
  const actions = new Map<string, (typeof result)[number]>();
  for (const message of result)
    for (const ref of message.pendingActionRefs) {
      const id = key(message.conversationId, ref);
      if (!actions.has(id)) actions.set(id, message);
      else
        message.pendingActionRefs = message.pendingActionRefs.filter(
          (value) => value !== ref,
        );
    }
  for (const message of result)
    for (const call of [...message.toolCalls]) {
      const ref =
        call.display?.mode === "action" ? call.display.actionRef : undefined;
      if (!ref) continue;
      const id = key(message.conversationId, ref);
      let owner = actions.get(id);
      if (!owner) {
        owner = message;
        owner.pendingActionRefs.push(ref);
        actions.set(id, owner);
      }
      if (owner === message) continue;
      if (!owner.toolCalls.some((item) => item.callId === call.callId))
        owner.toolCalls.push(call);
      if (
        message.presentations.includes(call.callId) &&
        !owner.presentations.includes(call.callId)
      )
        owner.presentations.push(call.callId);
      message.presentations = message.presentations.filter(
        (value) => value !== call.callId,
      );
      message.toolCalls = message.toolCalls.filter(
        (value) => value.callId !== call.callId,
      );
    }
  const viewed = new Set<string>();
  for (const message of result)
    message.presentations = message.presentations.filter((id) => {
      const group = message.toolCalls.find((call) => call.callId === id)
        ?.display?.resultKey;
      if (!group) return true;
      const identity = key(message.conversationId, group);
      if (viewed.has(identity)) return false;
      viewed.add(identity);
      return true;
    });
  return result.filter(hasChatContent);
}
