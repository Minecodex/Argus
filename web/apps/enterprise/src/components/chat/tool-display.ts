export type ToolDisplay = {
  mode: "template" | "trace" | "action" | "file";
  actionRef?: string;
  resultKey?: string;
  facts: { key: string; value: string }[];
  issues: { path: string; message: string; severity: string }[];
};
const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
export function parseToolDisplay(value: unknown): ToolDisplay | undefined {
  const data = record(value);
  if (!["template", "trace", "action", "file"].includes(String(data.mode)))
    return;
  return {
    resultKey:
      typeof data.result_key === "string" ? data.result_key : undefined,
    mode: data.mode as ToolDisplay["mode"],
    actionRef:
      typeof data.action_ref === "string" ? data.action_ref : undefined,
    facts: (Array.isArray(data.facts) ? data.facts : []).flatMap((value) => {
      const row = record(value);
      return typeof row.key === "string" && typeof row.value === "string"
        ? [{ key: row.key, value: row.value }]
        : [];
    }),
    issues: (Array.isArray(data.issues) ? data.issues : []).map((value) => {
      const row = record(value);
      return {
        path: String(row.path ?? ""),
        message: String(row.message ?? ""),
        severity: String(row.severity ?? ""),
      };
    }),
  };
}

/** Old native connect events recorded the authoritative action reference in
 * resource_refs before their provider supplied explicit display metadata. */
export function parseToolEventDisplay(
  payload: Record<string, unknown>,
): ToolDisplay | undefined {
  const explicit = parseToolDisplay(payload.display);
  if (explicit) return explicit;
  if (
    payload.source !== "argus" ||
    (payload.target_tool_id ?? payload.tool_id) !== "connect"
  )
    return;
  const projection = record(payload.projection);
  const refs = (
    Array.isArray(projection.resource_refs) ? projection.resource_refs : []
  )
    .map(record)
    .filter((ref) => ref.kind === "action_ref" && typeof ref.ref === "string");
  if (refs.length !== 1) return;
  return {
    mode: "action",
    actionRef: refs[0]!.ref as string,
    facts: [],
    issues: [],
  };
}
export function toolDisplaySummary(
  display: ToolDisplay | undefined,
  t: (key: string, options: { value: string }) => string,
) {
  const labels: Record<string, string> = {
    returned: "chat.resultSummary.returned",
    resources: "chat.resultSummary.resources",
    from: "chat.resultSummary.from",
    to: "chat.resultSummary.to",
    validation_issues: "chat.resultSummary.validationIssues",
    sample_status: "chat.resultSummary.sampleStatus",
    partial: "chat.resultSummary.partial",
  };
  return display?.facts
    .map(({ key, value }) => (labels[key] ? t(labels[key], { value }) : value))
    .join(" · ");
}
