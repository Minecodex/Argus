import { Button, DataTable } from "@argus/ui";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown, LoaderCircle, Wrench, X } from "lucide-react";
import type { ToolCallTrace } from "./chat-view-model";
import { toolDetailsText } from "./tool-details";
import { toolDisplayName } from "./tool-name";
import { toolDisplaySummary } from "./tool-display";

function formatDuration(durationMs?: number): string {
  if (durationMs === undefined) return "";
  return durationMs >= 1000
    ? `${(durationMs / 1000).toFixed(1)}s`
    : `${durationMs}ms`;
}

function StatusIcon({ status }: { status: ToolCallTrace["status"] }) {
  if (status === "running") {
    return (
      <span className="argus-chat-trace__status is-running">
        <LoaderCircle aria-hidden className="argus-spin" size={13} />
      </span>
    );
  }
  if (status === "failed") {
    return (
      <span className="argus-chat-trace__status is-failed">
        <X aria-hidden size={13} />
      </span>
    );
  }
  return (
    <span className="argus-chat-trace__status is-success">
      <Check aria-hidden size={13} />
    </span>
  );
}

/** 工具调用 trace：默认折叠的可展开区块（名称、参数摘要、状态、耗时）。 */
export function ToolTrace({
  toolCalls,
  label,
}: {
  toolCalls: ToolCallTrace[];
  label?: string;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  if (toolCalls.length === 0) return null;
  const single = toolCalls.length === 1 ? toolCalls[0] : undefined;

  return (
    <div className="argus-chat-trace" data-testid="tool-trace">
      <Button
        variant="ghost"
        aria-expanded={expanded}
        aria-label={
          single
            ? t(
                expanded ? "chat.trace.collapseTool" : "chat.trace.expandTool",
                {
                  tool: toolDisplayName(t, single.toolName, single.source),
                },
              )
            : expanded
              ? t("chat.trace.collapse")
              : t("chat.trace.expand")
        }
        className="argus-chat-trace__toggle"
        onPress={() => setExpanded((value) => !value)}
        type="button"
      >
        <Wrench aria-hidden size={12} />
        <span>
          {label ??
            (single
              ? toolDisplayName(t, single.toolName, single.source)
              : t("chat.trace.title", { count: toolCalls.length }))}
        </span>
        {single && (
          <>
            <StatusIcon status={single.status} />
            <span className="argus-chat-trace__summary">
              {toolDisplaySummary(single.display, t) || single.summary}
            </span>
            <time>{formatDuration(single.durationMs)}</time>
          </>
        )}
        <ChevronDown aria-hidden size={13} />
      </Button>
      {expanded && (
        <div className="argus-chat-trace__list">
          {toolCalls.map((call) => (
            <div className="argus-chat-trace__step" key={call.callId}>
              <div className="argus-chat-trace__row">
                <StatusIcon status={call.status} />
                <span>{toolDisplayName(t, call.toolName, call.source)}</span>
                <small>{t(`chat.trace.${call.status}`)}</small>
                <time>{formatDuration(call.durationMs)}</time>
              </div>
              {toolDisplayName(t, call.toolName, call.source) !==
                call.toolName && (
                <small className="argus-chat-trace__identifier">
                  {t("toolNames.identifier")}: <code>{call.toolName}</code>
                </small>
              )}
              {call.summary && <p>{call.summary}</p>}
              {!!call.display?.issues.length && (
                <DataTable
                  data={call.display.issues}
                  getRowKey={(row) => row.path + row.message}
                  columns={[
                    { key: "path", header: t("chat.trace.issuePath") },
                    { key: "severity", header: t("chat.trace.issueSeverity") },
                    { key: "message", header: t("chat.trace.issueMessage") },
                  ]}
                />
              )}
              {call.input !== undefined && (
                <details>
                  <summary>{t("chat.trace.input")}</summary>
                  <pre>{toolDetailsText(call.input)}</pre>
                </details>
              )}
              {call.output !== undefined && (
                <details>
                  <summary>{t("chat.trace.output")}</summary>
                  <pre>{toolDetailsText(call.output)}</pre>
                </details>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
