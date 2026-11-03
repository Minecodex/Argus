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
import { resolveObservationResources } from "./observation-access";
import { resolvePermissions } from "./permissions";

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
      task_scope: "general_diagnosis",
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
  const text = input.content.trim();
  const command = /^\/(?:创建仪表盘|create-dashboard)(?:\s|$)/.test(text);
  const analysis =
    /^\/(?:分析仪表盘|analyze-dashboard)(?:\s|$)/.test(text) ||
    /^(?:只分析|仅分析)[^\n]*仪表盘/.test(text) ||
    /^only analy[sz]e [^\n]*dashboard/i.test(text);
  const reset = /^\/(?:综合诊断|diagnose)(?:\s|$)/.test(text);
  let selected =
    input.dashboard_context ??
    (command ? { mode: "create" as const, dashboard_ids: [] } : current);
  if (command || analysis || reset)
    selected = {
      ...selected,
      mode: command ? "create" : analysis ? "analyze" : "none",
      dashboard_ids: reset ? [] : selected.dashboard_ids,
    };
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
  const refs = selected.resource_refs ?? [];
  if (
    refs.length > 20 ||
    new Set(refs.map((ref) => `${ref.type}:${ref.id}`)).size !== refs.length
  )
    dashboardFailure();
  for (const ref of refs) {
    if (ref.type === "dashboard") mockDashboard(ctx, ref.id);
    else if (ref.type === "connector") {
      const permissions = resolvePermissions(
        ctx.db,
        ctx.enterpriseId(),
        ctx.actor().id,
        ctx.nowIso(),
      );
      if (!permissions.has("*") && !permissions.has("connector.read"))
        dashboardFailure("DASHBOARD_DENIED", 403);
      if (
        !ctx.db.connectors.some(
          (row) => row.id === ref.id && row.enterpriseId === ctx.enterpriseId(),
        )
      )
        dashboardFailure("DASHBOARD_DENIED", 403);
    } else
      resolveObservationResources(ctx, {
        mode: "explicit",
        resources: [{ type: ref.type, id: ref.id }],
      });
  }
  const changed =
    selected.mode !== current.mode ||
    JSON.stringify(selected.dashboard_ids) !==
      JSON.stringify(current.dashboard_ids) ||
    JSON.stringify(refs) !== JSON.stringify(current.resource_refs ?? []);
  return {
    schema_version: "argus.dashboard_context/v1",
    mode: selected.mode,
    dashboard_ids: [...selected.dashboard_ids],
    resource_refs: structuredClone(refs),
    task_scope:
      selected.mode === "create"
        ? "dashboard_authoring"
        : selected.mode === "analyze"
          ? "dashboard_analysis"
          : "general_diagnosis",
    version: current.version + Number(changed),
  };
}
