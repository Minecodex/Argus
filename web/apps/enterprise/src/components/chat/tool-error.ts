import type { TFunction } from "i18next";
import type { ToolCallTrace } from "./chat-view-model";

const object = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

/** Requested identity is text, never proof of authorization or successful routing. */
export function requestedToolLabel(name: string | undefined, input: unknown) {
  if (!name || !["tool.invoke", "tool.describe"].includes(name)) return;
  const data = object(input);
  return typeof data.category === "string" &&
    typeof data.name === "string" &&
    /^[a-zA-Z0-9_.-]{1,128}$/.test(data.category) &&
    /^[a-zA-Z0-9_.-]{1,128}$/.test(data.name)
    ? `${data.category}/${data.name}`
    : undefined;
}

export function toolFailureText(t: TFunction, call: ToolCallTrace) {
  if (
    call.status !== "failed" ||
    (call.source && !["argus", "sandbox"].includes(call.source))
  )
    return;
  const data = object(call.output);
  const code =
    typeof data.error_code === "string" &&
    /^[A-Z][A-Z0-9_]{0,95}$/.test(data.error_code)
      ? data.error_code
      : undefined;
  if (!code) return;
  const labels: Record<string, string> = {
    TOOL_SEARCH_REQUIRED: "chat.toolFailure.searchRequired",
    TOOL_DESCRIPTION_REQUIRED: "chat.toolFailure.descriptionRequired",
    TOOL_INPUT_INVALID: "chat.toolFailure.invalidInput",
    DASHBOARD_VERSION_CONFLICT: "chat.toolFailure.versionConflict",
    DASHBOARD_CONDITION_EVIDENCE_REQUIRED: "chat.toolFailure.conditionEvidence",
    DASHBOARD_RESOURCE_SCOPE_REQUIRED: "chat.toolFailure.resourceScope",
    WORKSPACE_FILE_NOT_FOUND: "chat.toolFailure.fileNotFound",
    TOOL_RESULT_UNKNOWN: "chat.toolFailure.unknownResult",
    TOOL_PERMISSION_DENIED: "chat.toolFailure.permissionDenied",
  };
  const detail = object(data.details);
  const version =
    typeof detail.actual_version === "number"
      ? detail.actual_version
      : undefined;
  return labels[code]
    ? t(labels[code]!, { version: version ?? "—" })
    : t("chat.toolFailure.other", { code });
}

export function toolFailureIssues(t: TFunction, call: ToolCallTrace) {
  if (
    call.status !== "failed" ||
    (call.source && !["argus", "sandbox"].includes(call.source))
  )
    return [];
  const details = object(object(call.output).details);
  return (
    Array.isArray(details.issues) ? details.issues.slice(0, 16) : []
  ).flatMap((value) => {
    const issue = object(value);
    if (typeof issue.path !== "string" || issue.path.length > 512) return [];
    const rule =
      typeof issue.constraint === "string" ? issue.constraint : "schema";
    const labels: Record<string, string> = {
      exact_excerpt_from_current_user_message: "chat.toolFailure.exactQuote",
      one_entry_per_changed_condition: "chat.toolFailure.oneEvidence",
      unique_changed_condition_path: "chat.toolFailure.uniquePath",
      nonempty_reason_and_valid_optional_quote:
        "chat.toolFailure.selectionReason",
      required: "chat.toolFailure.requiredField",
      additionalProperties: "chat.toolFailure.unexpectedField",
      type: "chat.toolFailure.fieldType",
    };
    return [
      {
        path: issue.path,
        severity: t("chat.toolFailure.error"),
        message: labels[rule] ? t(labels[rule]!) : rule,
      },
    ];
  });
}
