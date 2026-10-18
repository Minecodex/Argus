import { useTranslation } from "react-i18next";
import { Bot } from "lucide-react";
import { Badge, Markdown } from "@argus/ui";
import { useEnterpriseAuthStore } from "@argus/auth";
import { PendingActionCard } from "./pending-action-card";
import { ConversationInteractionItem } from "./conversation-interaction";
import { ToolTrace } from "./tool-trace";
import { ConversationToolResult } from "./tool-result";
import { MessageFiles } from "./conversation-file";
import type { ChatMessage } from "./chat-view-model";
import { hasChatContent } from "./chat-content";

function formatTime(value: string, locale: string): string {
  return new Date(value).toLocaleTimeString(locale, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function ChatAssistantHeading({ message }: { message: ChatMessage }) {
  const { t, i18n } = useTranslation();
  return (
    <div
      className="argus-chat-message__who"
      data-testid="chat-assistant-heading"
    >
      <span className="argus-chat-message__avatar">
        <Bot aria-hidden size={14} />
      </span>
      <b>{t("chat.assistant")}</b>
      {message.modelId && <Badge tone="accent">{message.modelId}</Badge>}
      <time>{formatTime(message.createdAt, i18n.language)}</time>
    </div>
  );
}

/** 会话消息与独立的工具详情、宿主确认和文件交付。 */
export function ChatMessageItem({
  message,
  streaming = false,
  showAssistantHeading = true,
}: {
  message: ChatMessage;
  /** 流式生成中：内容末尾显示光标。 */
  streaming?: boolean;
  showAssistantHeading?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const downloads = (items: ChatMessage["files"], kind: "file" | "delivery") =>
    items?.length ? (
      <MessageFiles
        items={items}
        conversationId={message.conversationId}
        kind={kind}
      />
    ) : null;
  const user = useEnterpriseAuthStore((state) => state.session?.user);
  if (message.interactionId)
    return (
      <div
        className="argus-chat-activity"
        data-testid="chat-interaction-event"
        data-sequence={message.sequence}
      >
        <ConversationInteractionItem
          conversationId={message.conversationId}
          id={message.interactionId}
        />
      </div>
    );
  const unpresentedCalls =
    message.toolCalls?.filter(
      (call) =>
        !message.presentations?.includes(call.callId) &&
        !(
          call.display?.actionRef &&
          message.pendingActionRefs?.includes(call.display.actionRef)
        ),
    ) ?? [];
  const actionFor = (id: string) =>
    message.toolCalls?.find((call) => call.callId === id)?.display?.actionRef;
  const presentations = message.presentations
    ?.filter((id) => !message.pendingActionRefs?.includes(actionFor(id) ?? ""))
    .map((id) => (
      <ConversationToolResult
        key={id}
        conversationId={message.conversationId}
        toolCallId={id}
        call={message.toolCalls?.find((call) => call.callId === id)}
      />
    ));
  const actions = message.pendingActionRefs?.map((ref) => {
    const calls =
      message.toolCalls?.filter((call) => call.display?.actionRef === ref) ??
      [];
    const results =
      message.presentations?.filter((id) => actionFor(id) === ref) ?? [];
    return (
      <PendingActionCard key={ref} actionRef={ref}>
        {results.map((id) => (
          <ConversationToolResult
            key={id}
            conversationId={message.conversationId}
            toolCallId={id}
            call={calls.find((call) => call.callId === id)}
          />
        ))}
        {calls.some((call) => !results.includes(call.callId)) && (
          <ToolTrace
            label={t("chat.trace.details")}
            toolCalls={calls.filter((call) => !results.includes(call.callId))}
          />
        )}
      </PendingActionCard>
    );
  });
  if (!hasChatContent(message)) return null;
  const fileOnly =
    !message.content.trim() &&
    Boolean(message.files?.length || message.artifacts?.length) &&
    !message.toolCalls?.length &&
    !message.presentations?.length &&
    !message.pendingActionRefs?.length;
  if ((message.kind && message.kind !== "message") || fileOnly) {
    return (
      <div
        className="argus-chat-activity"
        data-testid={`chat-${message.kind && message.kind !== "message" ? message.kind : "file"}-event`}
        data-sequence={message.sequence}
      >
        {message.kind === "tool" && <ToolTrace toolCalls={unpresentedCalls} />}
        {(message.kind === "tool" || message.kind === "presentation") &&
          presentations}
        {actions}
        {message.files?.length ? (
          <MessageFiles
            items={message.files}
            conversationId={message.conversationId}
            group={message.fileGroup}
          />
        ) : null}
        {message.artifacts?.length ? (
          <MessageFiles
            items={message.artifacts}
            conversationId={message.conversationId}
            kind="delivery"
          />
        ) : null}
      </div>
    );
  }

  if (message.role === "user") {
    return (
      <div
        className="argus-chat-message argus-chat-message--user"
        data-testid="chat-message-user"
      >
        <div className="argus-chat-message__who">
          <b>{user?.display_name ?? t("chat.you")}</b>
          <time>{formatTime(message.createdAt, i18n.language)}</time>
        </div>
        {message.content.trim() && (
          <div className="argus-chat-message__body">{message.content}</div>
        )}
        {downloads(message.files, "file")}
      </div>
    );
  }

  return (
    <div className="argus-chat-message" data-testid="chat-message-assistant">
      {showAssistantHeading && <ChatAssistantHeading message={message} />}
      {message.content.trim() && (
        <div
          className={`argus-chat-message__body ${streaming && message.content ? "argus-chat-message__caret" : ""}`}
        >
          <Markdown>{message.content}</Markdown>
        </div>
      )}
      {Boolean(
        message.toolCalls?.length ||
        message.presentations?.length ||
        message.pendingActionRefs?.length ||
        message.files?.length ||
        message.artifacts?.length,
      ) && (
        <div className="argus-chat-message__extras">
          {unpresentedCalls.length > 0 && (
            <ToolTrace toolCalls={unpresentedCalls} />
          )}
          {presentations}
          {actions}
          {downloads(message.artifacts, "delivery")}
          {downloads(message.files, "file")}
        </div>
      )}
    </div>
  );
}
