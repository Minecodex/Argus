import type { ChatMessage } from "./chat-view-model";
import { hasChatContent } from "./chat-content";

export type ChatReplyGroup = {
  id: string;
  role: "user" | "assistant";
  entries: ChatMessage[];
};

/** User input is the visual reply boundary; tool, file and Run boundaries are not. */
export function groupChatReplies(
  timeline: readonly ChatMessage[],
): ChatReplyGroup[] {
  const groups: ChatReplyGroup[] = [];
  let userId = "initial";
  for (const entry of timeline) {
    if (!hasChatContent(entry)) continue;
    if (entry.role === "user" && (!entry.kind || entry.kind === "message")) {
      userId = entry.id;
      groups.push({ id: entry.id, role: "user", entries: [entry] });
      continue;
    }
    const previous = groups[groups.length - 1];
    if (previous?.role === "assistant") previous.entries.push(entry);
    else
      groups.push({
        id: `${entry.conversationId}:${userId}:reply`,
        role: "assistant",
        entries: [entry],
      });
  }
  return groups;
}
