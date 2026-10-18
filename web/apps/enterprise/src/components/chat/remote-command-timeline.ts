import type { ChatMessage } from "./chat-view-model";
import { hasChatContent } from "./chat-content";

/** Consent and outcome update one connect activity at its original event
 * position. A plan's identity never determines where command records appear. */
export function coalesceRemoteCommands(
  source: readonly ChatMessage[],
): ChatMessage[] {
  const entries = source.map((entry) => ({
    ...entry,
    toolCalls: entry.toolCalls ? [...entry.toolCalls] : undefined,
    pendingActionRefs: entry.pendingActionRefs
      ? [...entry.pendingActionRefs]
      : undefined,
  }));
  const refs = new Set(
    entries.flatMap(
      (entry) =>
        entry.toolCalls
          ?.filter(
            (call) => call.toolName === "connect" && call.display?.actionRef,
          )
          .map((call) => call.display!.actionRef!) ?? [],
    ),
  );
  for (const ref of refs) {
    const related = entries
      .map((entry, index) => ({ entry, index }))
      .filter(
        ({ entry }) =>
          entry.pendingActionRefs?.includes(ref) ||
          entry.toolCalls?.some(
            (call) =>
              call.toolName === "connect" && call.display?.actionRef === ref,
          ),
      );
    const target = related[0];
    if (!target) continue;
    const trace = related
      .flatMap(({ entry }) => entry.toolCalls ?? [])
      .reverse()
      .find(
        (call) =>
          call.toolName === "connect" && call.display?.actionRef === ref,
      );
    for (const { entry } of related) {
      entry.toolCalls = entry.toolCalls?.filter(
        (call) =>
          !(call.toolName === "connect" && call.display?.actionRef === ref),
      );
      entry.pendingActionRefs = entry.pendingActionRefs?.filter(
        (id) => id !== ref,
      );
    }
    target.entry.pendingActionRefs = [
      ...(target.entry.pendingActionRefs ?? []),
      ref,
    ];
    if (trace)
      target.entry.toolCalls = [...(target.entry.toolCalls ?? []), trace];
    if (target.entry.kind === "tool") target.entry.kind = "action";
  }
  return entries.filter(hasChatContent);
}
