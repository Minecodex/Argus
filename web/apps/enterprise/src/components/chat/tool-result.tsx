import { useTranslation } from "react-i18next";
import { ToolResultDisclosure } from "@argus/ui";
import { ConversationPresentation } from "./tool-presentation";
import { toolDetailsText } from "./tool-details";
import { toolDisplayName } from "./tool-name";
import { toolDisplaySummary } from "./tool-display";
import type { ToolCallTrace } from "./chat-view-model";

function ToolResultData({
  value,
  toolName,
  emptyMessage,
}: {
  value: unknown;
  toolName?: string;
  emptyMessage: string;
}) {
  return (
    <div className="argus-chat-result-data">
      {toolName && (
        <p className="argus-chat-result-data__tool">
          <code>{toolName}</code>
        </p>
      )}
      {value === undefined ? (
        <p>{emptyMessage}</p>
      ) : (
        <pre tabIndex={0}>{toolDetailsText(value)}</pre>
      )}
    </div>
  );
}

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
  const title = call
    ? toolDisplayName(t, call.toolName, call.source)
    : t("planv5.template.title");
  // Only display an explicit public summary. Never guess names, counts or units from IDs.
  const summary =
    toolDisplaySummary(call?.display, t) ||
    call?.summary ||
    (typeof call?.output === "string" ? call.output : undefined);
  const duration =
    call?.durationMs === undefined
      ? undefined
      : call.durationMs >= 1000
        ? `${(call.durationMs / 1000).toFixed(1)}s`
        : `${call.durationMs}ms`;
  return (
    <ToolResultDisclosure
      title={title}
      summary={summary}
      status={call?.status}
      statusLabel={call ? t(`chat.trace.${call.status}`) : undefined}
      duration={duration}
      viewLabel={t("chat.trace.viewResult")}
      collapseLabel={t("chat.trace.hideResult")}
      views={[
        {
          key: "view",
          label: t("chat.resultViews.view"),
          content: (
            <ConversationPresentation
              conversationId={conversationId}
              toolCallId={toolCallId}
              toolName={call?.toolName}
              source={call?.source}
            />
          ),
        },
        {
          key: "output",
          label: t("chat.trace.output"),
          content: (
            <ToolResultData
              value={call?.output}
              toolName={call?.toolName}
              emptyMessage={t("chat.resultViews.noOutput")}
            />
          ),
        },
        {
          key: "input",
          label: t("chat.resultViews.parameters"),
          content: (
            <ToolResultData
              value={call?.input}
              toolName={call?.toolName}
              emptyMessage={t("chat.resultViews.noInput")}
            />
          ),
        },
      ]}
    />
  );
}
