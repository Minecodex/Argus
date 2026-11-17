import type { ChatMessage, ToolCallTrace } from "./chat-view-model";
import { activityStatus } from "./tool-activity-model";
import type { ToolActivityStatus } from "@argus/ui";

export type ChatActivitySegment =
  | { kind: "message"; id: string; message: ChatMessage }
  | {
      kind: "tools";
      id: string;
      entries: ChatMessage[];
      calls: ToolCallTrace[];
    };
const isTool = (message: ChatMessage) =>
  message.role === "assistant" &&
  !message.content.trim() &&
  !message.interactionId &&
  !message.pendingActionRefs?.length &&
  !message.files?.length &&
  !message.artifacts?.length &&
  !!(message.toolCalls?.length || message.presentations?.length);

/** Pure view projection. Prose, approvals, files and Run boundaries are barriers. */
export function groupActivitySegments(
  messages: readonly ChatMessage[],
): ChatActivitySegment[] {
  const result: ChatActivitySegment[] = [];
  for (const raw of messages) {
    const pieces =
      raw.role === "assistant" &&
      raw.content.trim() &&
      (raw.toolCalls?.length || raw.presentations?.length) &&
      !raw.pendingActionRefs?.length
        ? [
            {
              ...raw,
              toolCalls: [],
              presentations: [],
              files: [],
              artifacts: [],
            },
            {
              ...raw,
              id: `${raw.id}-activities`,
              content: "",
              kind: "tool" as const,
            },
          ]
        : [raw];
    for (const message of pieces) {
      if (!isTool(message)) {
        result.push({ kind: "message", id: message.id, message });
        continue;
      }
      const last = result.at(-1),
        previous = last?.kind === "tools" ? last.entries.at(-1) : undefined;
      if (
        last?.kind === "tools" &&
        previous?.conversationId === message.conversationId &&
        previous.runId === message.runId
      ) {
        last.entries.push(message);
        last.calls.push(...(message.toolCalls ?? []));
      } else
        result.push({
          kind: "tools",
          id: `${message.conversationId}:${message.toolCalls?.[0]?.callId ?? message.id}`,
          entries: [message],
          calls: [...(message.toolCalls ?? [])],
        });
    }
  }
  return result;
}

export function groupActivityStatus(
  calls: readonly ToolCallTrace[],
): ToolActivityStatus {
  const states = calls.map(activityStatus);
  for (const state of [
    "running",
    "queued",
    "unknown",
    "failed",
    "cancelled",
  ] as const)
    if (states.includes(state)) return state;
  return "success";
}
