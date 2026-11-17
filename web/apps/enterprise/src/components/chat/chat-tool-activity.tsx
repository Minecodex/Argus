import { DataTable, DiffViewer, ToolActivityRow } from "@argus/ui";
import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import type { ToolCallTrace } from "./chat-view-model";
import {
  activityDuration,
  activityKind,
  activityPath,
  activityRecord,
  activityStatus,
} from "./tool-activity-model";
import { toolTraceName } from "./tool-name";
import { toolFailureIssues, toolFailureText } from "./tool-error";
import { toolDisplaySummary } from "./tool-display";
import { toolDetailsText } from "./tool-details";

export function ChatToolActivity({
  call,
  label,
  wide,
  children,
}: {
  call: ToolCallTrace;
  label?: string;
  wide?: boolean;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const kind = activityKind(call),
    state = activityStatus(call),
    path = activityPath(call);
  const input = activityRecord(call.input),
    output = activityRecord(call.output);
  const change = call.activity?.change;
  const issues = call.display?.issues.length
    ? call.display.issues
    : toolFailureIssues(t, call);
  const title =
    label ??
    (kind === "tool"
      ? toolTraceName(t, call)
      : t(
          `chat.activity.${kind === "file_change" && change?.operation === "create" ? "file_create" : kind}`,
        ));
  const source =
    call.source === "argus"
      ? t("chat.activity.platform")
      : call.source === "sandbox"
        ? t("chat.activity.workspace")
        : call.source
          ? t("chat.activity.external")
          : t("chat.activity.legacy");
  const summary =
    toolFailureText(t, call) ||
    path ||
    (kind === "command" && typeof input.command === "string"
      ? input.command
      : undefined) ||
    toolDisplaySummary(call.display, t) ||
    call.summary;
  return (
    <ToolActivityRow
      title={title}
      summary={summary}
      kind={kind}
      status={state}
      statusLabel={t(`chat.activity.states.${state}`)}
      duration={activityDuration(call)}
      wide={wide}
      change={
        change?.added !== undefined && change.removed !== undefined
          ? { added: change.added, removed: change.removed }
          : undefined
      }
    >
      <div className="argus-tool-activity__provenance">
        <span>{source}</span>
        <span>
          {t("toolNames.identifier")}: <code>{call.toolName}</code>
        </span>
        <span>{t(`chat.activity.states.${state}`)}</span>
      </div>
      {path && (
        <p>
          <code>{path}</code>
        </p>
      )}
      {(state === "failed" || state === "unknown") && (
        <p role="alert">
          {toolFailureText(t, call) ||
            (kind === "command" && typeof output.exit_code === "number"
              ? t("chat.activity.exitCode", { code: output.exit_code })
              : t(`chat.activity.states.${state}`))}
        </p>
      )}
      {kind === "file_change" && (
        <>
          {change?.lines.length ? <DiffViewer lines={change.lines} /> : null}
          {(!change || !change.complete) && (
            <p>
              {t(
                `chat.activity.${change ? "partialDiff" : "noHistoricalDiff"}`,
              )}
            </p>
          )}
        </>
      )}
      {kind === "file_read" && typeof output.content === "string" && (
        <pre tabIndex={0}>{output.content}</pre>
      )}
      {kind === "command" && (
        <>
          {typeof input.command === "string" && (
            <pre tabIndex={0}>{input.command}</pre>
          )}
          {typeof output.exit_code === "number" && (
            <p>{t("chat.activity.exitCode", { code: output.exit_code })}</p>
          )}
          {typeof output.stdout === "string" && output.stdout && (
            <pre tabIndex={0}>{output.stdout}</pre>
          )}
          {typeof output.stderr === "string" && output.stderr && (
            <pre tabIndex={0}>{output.stderr}</pre>
          )}
          {output.partial === true && <p>{t("chat.activity.partialOutput")}</p>}
        </>
      )}
      {issues.length ? (
        <DataTable
          data={issues}
          getRowKey={(row) => row.path + row.message}
          surface="plain"
          columns={[
            { key: "path", header: t("chat.trace.issuePath") },
            { key: "severity", header: t("chat.trace.issueSeverity") },
            { key: "message", header: t("chat.trace.issueMessage") },
          ]}
        />
      ) : null}
      {children}
      {call.input !== undefined && (
        <details>
          <summary>{t("chat.trace.input")}</summary>
          <pre tabIndex={0}>{toolDetailsText(call.input)}</pre>
        </details>
      )}
      {call.output !== undefined && (
        <details>
          <summary>{t("chat.trace.output")}</summary>
          <pre tabIndex={0}>{toolDetailsText(call.output)}</pre>
        </details>
      )}
    </ToolActivityRow>
  );
}
