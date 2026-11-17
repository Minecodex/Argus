import { useTranslation } from "react-i18next";
import { useState } from "react";
import { ConversationPresentation } from "./tool-presentation";
import { ChatToolActivity } from "./chat-tool-activity";
import type { ToolCallTrace } from "./chat-view-model";
export function ConversationToolResult({
  conversationId,
  toolCallId,
  call,
}: {
  conversationId: string;
  toolCallId: string;
  call?: ToolCallTrace;
}) {
  const { t } = useTranslation();
  const [viewOpen, setViewOpen] = useState(false);
  return (
    <div data-testid="tool-result" className="argus-chat-tool-result">
      <ChatToolActivity
        wide={viewOpen}
        call={
          call ?? {
            callId: toolCallId,
            toolName: t("planv5.template.title"),
            status: "success",
            startedAt: "",
          }
        }
      >
        <details
          data-testid="tool-business-view"
          open={viewOpen}
          onToggle={(event) => setViewOpen(event.currentTarget.open)}
        >
          <summary>{t("chat.resultViews.view")}</summary>
          {viewOpen && (
            <ConversationPresentation
              conversationId={conversationId}
              toolCallId={toolCallId}
              toolName={call?.toolName}
              source={call?.source}
            />
          )}
        </details>
      </ChatToolActivity>
    </div>
  );
}
