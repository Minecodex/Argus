import { useTranslation } from "react-i18next";
import { Bot } from "lucide-react";
import { Avatar, Badge } from "@argus/ui";
import { useEnterpriseAuthStore } from "@argus/auth";
import { PendingActionCard } from "./pending-action-card";
import { ToolTrace } from "./tool-trace";
import { ConversationPresentation } from "./tool-presentation";
import { useApi } from "@argus/api-client";
import type { ChatMessage } from "./chat-view-model";

function formatTime(value: string, locale: string): string {
  return new Date(value).toLocaleTimeString(locale, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** 会话消息与独立的工具详情、宿主确认和文件交付。 */
export function ChatMessageItem({
  message,
  streaming = false,
}: {
  message: ChatMessage;
  /** 流式生成中：内容末尾显示光标。 */
  streaming?: boolean;
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

  if (message.role === "user") {
    return (
      <div
        className="argus-chat-message argus-chat-message--user"
        data-testid="chat-message-user"
      >
        <div className="argus-chat-message__who">
          <Avatar
            fallback={(user?.display_name ?? t("chat.you")).slice(0, 1)}
            size="sm"
          />
          <b>{user?.display_name ?? t("chat.you")}</b>
          <time>{formatTime(message.createdAt, i18n.language)}</time>
        </div>
        <div className="argus-chat-message__body">{message.content}</div>
        {downloads(message.files, "file")}
      </div>
    );
  }

  return (
    <div className="argus-chat-message" data-testid="chat-message-assistant">
      <div className="argus-chat-message__who">
        <span className="argus-chat-message__avatar">
          <Bot aria-hidden size={14} />
        </span>
        <b>{t("chat.assistant")}</b>
        {message.modelId && <Badge tone="accent">{message.modelId}</Badge>}
        <time>{formatTime(message.createdAt, i18n.language)}</time>
      </div>
      <div
        className={`argus-chat-message__body ${streaming && message.content ? "argus-chat-message__caret" : ""}`}
      >
        {message.content}
      </div>
      {(message.toolCalls?.length ||
        message.presentations?.length ||
        message.pendingActionRefs?.length ||
        message.artifacts?.length) && (
        <div className="argus-chat-message__extras">
          {message.toolCalls && message.toolCalls.length > 0 && (
            <ToolTrace toolCalls={message.toolCalls} />
          )}
          {message.presentations?.map((toolCallId) => (
            <ConversationPresentation
              key={toolCallId}
              conversationId={message.conversationId}
              toolCallId={toolCallId}
            />
          ))}
          {message.pendingActionRefs?.map((actionRef) => (
            <PendingActionCard actionRef={actionRef} key={actionRef} />
          ))}
          {downloads(message.artifacts, "delivery")}
        </div>
      )}
    </div>
  );
}

function MessageFiles({
  items,
  conversationId,
  kind,
}: {
  items: NonNullable<ChatMessage["files"]>;
  conversationId: string;
  kind: "file" | "delivery";
}) {
  const api = useApi();
  return (
    <>
      {items.map((file) => {
        let href: string | undefined;
        try {
          href = api.workspace.downloadUrl(conversationId, file.id, kind);
        } catch {
          href = undefined;
        }
        return (
          <a
            className="argus-chat-file"
            href={href}
            aria-disabled={!href}
            download={file.name}
            key={file.id}
          >
            {file.name}
          </a>
        );
      })}
    </>
  );
}
