import type {
  DashboardChatContext,
  MessageCreate,
} from "../generated/contracts";
import type { MockContext } from "./context";
import {
  dashboardFailure,
  dashboardPermission,
  mockDashboard,
} from "./dashboard-state";
import { ApiError } from "../transport/errors";
import { nextId } from "./store";
import type { MockChatStreamEvent } from "./chat-types";

// The UI fixture demonstrates context persistence, not real telemetry/model
// execution. Never fabricate a healthy conclusion or pretend files were read.
export async function* mockDashboardReply(
  ctx: MockContext,
  id: string,
  mode: "analyze" | "create",
): AsyncGenerator<MockChatStreamEvent> {
  let english = false;
  try {
    english = globalThis.localStorage?.getItem("argus.locale") === "en-US";
  } catch {
    /* non-browser fixture */
  }
  const content =
    mode === "create"
      ? english
        ? "Dashboard creation mode is active in this demo. A real model has not generated or published a dashboard."
        : "演示已启用仪表盘创建模式。本次未调用真实模型生成或发布仪表盘。"
      : english
        ? "This demo saved your dashboard selection. No real telemetry query or file analysis ran, so no health conclusion is available."
        : "演示已保存仪表盘选择。本次未执行真实遥测查询或文件分析，不能据此判断健康状态。";
  const message = {
    id: nextId(ctx.db, "msg"),
    conversationId: id,
    role: "assistant" as const,
    content,
    createdAt: ctx.nowIso(),
  };
  yield { type: "message_start", messageId: message.id };
  yield { type: "token", messageId: message.id, delta: content };
  ctx.db.messages.push(message);
  ctx.save();
  yield { type: "message_done", message };
}

export function currentDashboardContext(
  ctx: MockContext,
  id: string,
): DashboardChatContext {
  ctx.mustFind(
    ctx.db.conversations,
    (c) =>
      c.id === id &&
      c.enterpriseId === ctx.enterpriseId() &&
      c.createdBy === ctx.actor().id,
    "conversation",
  );
  return (
    ctx.db.planv5.dashboardContexts?.[id] ?? {
      schema_version: "argus.dashboard_context/v1",
      mode: "none",
      dashboard_ids: [],
      version: 0,
    }
  );
}
export function prepareDashboardContext(
  ctx: MockContext,
  id: string,
  input: MessageCreate,
): DashboardChatContext {
  const current = currentDashboardContext(ctx, id);
  const command = /^\/(?:创建仪表盘|create-dashboard)(?:\s|$)/.test(
    input.content.trim(),
  );
  const selected =
    input.dashboard_context ??
    (command ? { mode: "create" as const, dashboard_ids: [] } : current);
  if (command && selected.mode !== "create") dashboardFailure();
  if (
    "expected_version" in selected &&
    selected.expected_version !== undefined &&
    selected.expected_version !== current.version
  )
    throw new ApiError(
      {
        code: "TOOL_CONFIGURATION_CHANGED",
        message_key: "errors.planv5.tool_configuration_changed",
        request_id: crypto.randomUUID(),
        retryable: false,
      },
      409,
    );
  if (
    !["none", "analyze", "create"].includes(selected.mode) ||
    selected.dashboard_ids.length > 20 ||
    new Set(selected.dashboard_ids).size !== selected.dashboard_ids.length ||
    (selected.mode === "none" && selected.dashboard_ids.length)
  )
    dashboardFailure();
  if (selected.mode !== "none") dashboardPermission(ctx);
  if (selected.mode === "create") dashboardPermission(ctx, true);
  for (const id of selected.dashboard_ids) {
    if (mockDashboard(ctx, id).lifecycle !== "active")
      dashboardFailure("DASHBOARD_DENIED", 403);
  }
  const changed =
    selected.mode !== current.mode ||
    JSON.stringify(selected.dashboard_ids) !==
      JSON.stringify(current.dashboard_ids);
  return {
    schema_version: "argus.dashboard_context/v1",
    mode: selected.mode,
    dashboard_ids: [...selected.dashboard_ids],
    version: current.version + Number(changed),
  };
}
