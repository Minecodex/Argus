import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { ChatAssistantHeading, ChatMessageItem } from "./message-item";
import type { ChatMessage } from "./chat-view-model";
import { mergeTimeline } from "./live-timeline";
import { groupChatReplies } from "./reply-groups";
import { combineToolActivities } from "./tool-activities";

/**
 * 消息流：历史消息 + 乐观用户消息 + 流式中的 AI 消息；
 * 流式期间自动滚动到底。
 */
export function ChatMessageList({
  messages,
  pendingUser,
  streaming,
}: {
  messages: ChatMessage[];
  pendingUser: ChatMessage | null;
  streaming: ChatMessage | null;
}) {
  const { t } = useTranslation();
  const streamRef = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const pendingUserPersisted =
    pendingUser !== null &&
    messages.some(
      (message) =>
        message.role === "user" &&
        message.content === pendingUser.content &&
        Math.abs(
          new Date(message.createdAt).getTime() -
            new Date(pendingUser.createdAt).getTime(),
        ) < 5_000,
    );
  const timeline = mergeTimeline(
    pendingUser && !pendingUserPersisted
      ? [...messages, pendingUser]
      : messages,
    streaming,
  );
  const replies = groupChatReplies(combineToolActivities(timeline));

  useEffect(() => {
    const el = streamRef.current;
    if (el && follow.current) el.scrollTop = el.scrollHeight;
  }, [messages, pendingUser, streaming]);

  return (
    <div
      className="argus-chat__stream"
      ref={streamRef}
      role="region"
      aria-label={t("chat.messageHistory")}
      tabIndex={0}
      onScroll={(event) => {
        const el = event.currentTarget;
        follow.current = el.scrollHeight - el.clientHeight - el.scrollTop < 80;
      }}
    >
      <div className="argus-chat-stream__inner">
        {replies.map((reply) => {
          if (reply.role === "user")
            return (
              <ChatMessageItem key={reply.id} message={reply.entries[0]!} />
            );
          const heading = reply.entries.find(
            (entry) => entry.role === "assistant",
          );
          return (
            <div
              key={reply.id}
              className="argus-chat-reply"
              data-testid="chat-assistant-reply"
            >
              {heading && <ChatAssistantHeading message={heading} />}
              <div className="argus-chat-reply__content">
                {reply.entries.map((message) => (
                  <ChatMessageItem
                    key={message.id}
                    message={message}
                    streaming={message.isStreaming}
                    showAssistantHeading={false}
                  />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
