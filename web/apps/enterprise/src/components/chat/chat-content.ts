import type { ChatMessage } from "./chat-view-model";

/** Model and ledger events without visible content are not chat messages. */
export function hasChatContent(message: ChatMessage): boolean {
  return Boolean(
    message.content.trim() ||
    message.files?.length ||
    message.artifacts?.length ||
    message.toolCalls?.length ||
    message.presentations?.length ||
    message.pendingActionRefs?.length,
  );
}
