import type { ToolActivityKind, ToolActivityStatus } from "@argus/ui";
import type { ToolCallTrace } from "./chat-view-model";

export type ActivityEvidence = {
  schema: "argus.tool_activity/v1";
  kind: "file_read" | "file_change";
  path: string;
  workspace_id?: string;
  change?: {
    operation: string;
    before_hash?: string;
    after_hash?: string;
    added?: number;
    removed?: number;
    complete: boolean;
    reason?: string;
    lines: { type: "add" | "remove" | "context"; content: string }[];
  };
};
export const activityRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const count = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;

export function isInteractionTool(
  name: unknown,
  input: unknown,
  source: unknown,
): boolean {
  if (source && source !== "argus") return false;
  const args = activityRecord(input);
  return (
    name === "request_user_input" ||
    (name === "tool.invoke" &&
      args.category === "workflow" &&
      args.name === "plan.propose")
  );
}

/** Sideband authority comes from the recorded provider, never model/MCP data. */
export function parseActivity(
  value: unknown,
  source: unknown,
): ActivityEvidence | undefined {
  if (source !== "sandbox" && source !== "argus") return;
  const raw = activityRecord(value);
  if (
    raw.schema !== "argus.tool_activity/v1" ||
    !["file_read", "file_change"].includes(String(raw.kind)) ||
    typeof raw.path !== "string" ||
    raw.path.length > 4096
  )
    return;
  const change = activityRecord(raw.change);
  return {
    schema: "argus.tool_activity/v1",
    kind: raw.kind as ActivityEvidence["kind"],
    path: raw.path,
    workspace_id:
      typeof raw.workspace_id === "string" ? raw.workspace_id : undefined,
    ...(raw.kind === "file_change"
      ? {
          change: {
            operation: String(change.operation ?? "write"),
            complete: change.complete === true,
            before_hash:
              typeof change.before_hash === "string"
                ? change.before_hash
                : undefined,
            after_hash:
              typeof change.after_hash === "string"
                ? change.after_hash
                : undefined,
            added: count(change.added) ? change.added : undefined,
            removed: count(change.removed) ? change.removed : undefined,
            reason:
              typeof change.reason === "string" ? change.reason : undefined,
            lines: (Array.isArray(change.lines)
              ? change.lines.slice(0, 512)
              : []
            ).flatMap((value) => {
              const line = activityRecord(value);
              return ["add", "remove", "context"].includes(String(line.type)) &&
                typeof line.content === "string"
                ? [
                    {
                      type: line.type as "add" | "remove" | "context",
                      content: line.content.slice(0, 65536),
                    },
                  ]
                : [];
            }),
          },
        }
      : {}),
  };
}

export function activityStatus(call: ToolCallTrace): ToolActivityStatus {
  if (call.rawStatus === "result_unknown" || call.summary === "result_unknown")
    return "unknown";
  if (call.rawStatus === "cancelled") return "cancelled";
  if (call.rawStatus === "queued") return "queued";
  if (
    call.status === "success" &&
    call.source === "sandbox" &&
    call.toolName === "bash"
  ) {
    const code = activityRecord(call.output).exit_code;
    if (typeof code === "number" && code !== 0) return "failed";
  }
  return call.status;
}

/** A terminal Run is not proof that a dispatched side effect succeeded. */
export function settleUnfinishedActivity(call: ToolCallTrace): ToolCallTrace {
  return call.status === "running"
    ? {
        ...call,
        status: "failed",
        rawStatus: call.rawStatus === "queued" ? "cancelled" : "result_unknown",
      }
    : call;
}

export function activityKind(call: ToolCallTrace): ToolActivityKind {
  if (call.activity) return call.activity.kind;
  if (call.source === "sandbox") {
    if (call.toolName === "read") return "file_read";
    if (call.toolName === "write" || call.toolName === "edit")
      return "file_change";
    if (call.toolName === "bash") return "command";
  }
  return "tool";
}

export function activityPath(call: ToolCallTrace): string | undefined {
  if (activityKind(call) === "tool" || activityKind(call) === "command") return;
  const path =
    call.activity?.path ??
    activityRecord(call.output).path ??
    activityRecord(call.input).path;
  return typeof path === "string" ? path : undefined;
}

export function activityDuration(call: ToolCallTrace): string | undefined {
  const ms =
    call.durationMs ??
    (call.completedAt
      ? Date.parse(call.completedAt) - Date.parse(call.startedAt)
      : undefined);
  return ms !== undefined && Number.isFinite(ms) && ms >= 0
    ? ms >= 1000
      ? `${(ms / 1000).toFixed(1)}s`
      : `${ms}ms`
    : undefined;
}
