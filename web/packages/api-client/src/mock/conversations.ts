import type { ArgusApiClient } from "../client";
import type {
  AgentEvent,
  Conversation,
  ConversationEvent,
  StreamEventEnvelope,
} from "../generated/contracts";
import type { MockConversationRecord } from "./internal-types";
import type { MockChatMessage as ChatMessage } from "./chat-types";
import type { MockChatStreamEvent } from "./chat-types";
import type { MockContext } from "./context";
import { nextId } from "./store";
import {
  mockDashboardReply,
  currentDashboardContext,
  prepareDashboardContext,
} from "./dashboard-context";

function serializeMessage(message: ChatMessage): Record<string, unknown> {
  return {
    message_id: message.id,
    conversation_id: message.conversationId,
    role: message.role,
    content: message.content,
    dashboard_context: message.dashboardContext,
    created_at: message.createdAt,
    ...(message.modelId ? { model_id: message.modelId } : {}),
    ...(message.modelRevision ? { model_revision: message.modelRevision } : {}),
    ...(message.inputPricePerMillionSnapshot !== undefined
      ? {
          input_price_per_million_snapshot:
            message.inputPricePerMillionSnapshot,
        }
      : {}),
    ...(message.outputPricePerMillionSnapshot !== undefined
      ? {
          output_price_per_million_snapshot:
            message.outputPricePerMillionSnapshot,
        }
      : {}),
    ...(message.inputTokens !== undefined
      ? { input_tokens: message.inputTokens }
      : {}),
    ...(message.outputTokens !== undefined
      ? { output_tokens: message.outputTokens }
      : {}),
    tool_calls: (message.toolCalls ?? []).map((call) => ({
      call_id: call.callId,
      tool_name: call.toolName,
      status: call.status,
      ...(call.summary ? { summary: call.summary } : {}),
      ...(call.durationMs !== undefined
        ? { duration_ms: call.durationMs }
        : {}),
      started_at: call.startedAt,
    })),
    presentations: message.presentations ?? [],
    pending_action_refs: message.pendingActionRefs ?? [],
  };
}

function conversationEvent(
  enterpriseId: string,
  message: ChatMessage,
  sequence: number,
): ConversationEvent {
  return {
    schema_version: "argus.conversation_event/v1",
    event_id: `conversation-${message.id}`,
    sequence,
    enterprise_id: enterpriseId,
    conversation_id: message.conversationId,
    event_type: message.role === "user" ? "user_message" : "assistant_message",
    actor_type: message.role === "user" ? "user" : "model",
    occurred_at: message.createdAt,
    content_hash: `mock-content-${message.id}`.padEnd(64, "0"),
    data_classification: "internal",
    payload: { message: serializeMessage(message) },
  };
}

function publicConversation(value: MockConversationRecord): Conversation {
  return {
    id: value.id,
    title: value.title,
    selected_model_id: value.selectedModelId,
    selected_mcp_connection_ids: value.selectedMCPConnectionIds ?? [],
    status: value.status,
    version: value.version ?? 1,
    created_at: value.createdAt,
    updated_at: value.lastMessageAt,
  };
}

function agentEvent(
  runId: string,
  sequence: number,
  source: MockChatStreamEvent,
  occurredAt: string,
): AgentEvent {
  let event_type: AgentEvent["event_type"] = "message_delta";
  let payload: Record<string, unknown> = {};
  if (source.type === "message_start") {
    event_type = "message_started";
    payload = { message_id: source.messageId };
  } else if (source.type === "token") {
    payload = { message_id: source.messageId, delta: source.delta };
  } else if (source.type === "tool_call") {
    event_type = "tool_call_started";
    payload = {
      message_id: source.messageId,
      tool_call_id: source.toolCall.callId,
      tool_name: source.toolCall.toolName,
      started_at: source.toolCall.startedAt,
    };
  } else if (source.type === "tool_call_update") {
    event_type = "tool_call_completed";
    payload = {
      message_id: source.messageId,
      tool_call_id: source.callId,
      status: source.status,
      duration_ms: source.durationMs,
      ...(source.summary ? { summary: source.summary } : {}),
    };
  } else if (source.type === "presentation") {
    event_type = "tool_presentation";
    payload = { message_id: source.messageId, tool_call_id: source.toolCallId };
  } else if (source.type === "pending_action") {
    event_type = "pending_action_created";
    payload = { message_id: source.messageId, action_ref: source.actionRef };
  } else if (source.type === "message_done") {
    event_type = "message_completed";
    payload = {
      message_id: source.message.id,
      message: serializeMessage(source.message),
    };
  } else if (source.type === "error") {
    event_type = "run_failed";
    payload = { error_code: "MOCK_STREAM_FAILED", message: source.message };
  }
  return {
    schema_version: "argus.agent_event/v1",
    event_id: `${runId}-event-${sequence}`,
    sequence,
    run_id: runId,
    event_type,
    occurred_at: occurredAt,
    payload,
  };
}

async function* streamEnvelopes(
  source: AsyncIterable<MockChatStreamEvent>,
  signal?: AbortSignal,
): AsyncGenerator<StreamEventEnvelope> {
  const runId = `run-${crypto.randomUUID()}`;
  let sequence = 0;
  for await (const event of source) {
    if (signal?.aborted) return;
    sequence += 1;
    const agent = agentEvent(runId, sequence, event, new Date().toISOString());
    yield {
      schema_version: "argus.stream_event/v1",
      event_id: agent.event_id,
      sequence,
      event_type: "agent_event",
      occurred_at: agent.occurred_at,
      terminal: false,
      resume_cursor: agent.event_id,
      data: agent,
    };
    if (event.type === "message_done") {
      sequence += 1;
      const completed: AgentEvent = {
        schema_version: "argus.agent_event/v1",
        event_id: `${runId}-event-${sequence}`,
        sequence,
        run_id: runId,
        event_type: "run_completed",
        occurred_at: new Date().toISOString(),
        payload: { stop_reason: "completed" },
      };
      yield {
        schema_version: "argus.stream_event/v1",
        event_id: completed.event_id,
        sequence,
        event_type: "agent_event",
        occurred_at: completed.occurred_at,
        terminal: true,
        close_reason: "normal",
        resume_cursor: completed.event_id,
        data: completed,
      };
      return;
    }
  }
}

/** Conversations and streaming assistant replies. */
export function createConversationsDomain(
  ctx: MockContext,
): ArgusApiClient["conversations"] {
  const { db } = ctx;

  return {
    async remove(id) {
      const item = ctx.mustFind(
        db.conversations,
        (item) =>
          item.id === id &&
          item.enterpriseId === ctx.enterpriseId() &&
          item.createdBy === ctx.actor().id,
        "conversation",
      );
      item.status = "deleted";
      for (const file of db.planv5.files[id] ?? [])
        delete db.planv5.contents[file.id];
      delete db.planv5.files[id];
      delete db.planv5.workspaces[id];
      db.messages = db.messages.filter((item) => item.conversationId !== id);
      ctx.save();
    },
    async dashboardContext(id) {
      return structuredClone(currentDashboardContext(ctx, id));
    },
    async preflight(id, input) {
      prepareDashboardContext(ctx, id, input);
      const c = ctx.mustFind(
        db.conversations,
        (item) => item.id === id,
        "conversation",
      );
      const model = ctx.mustFind(
        db.models,
        (item) => item.id === c.selectedModelId,
        "model",
      );
      const selected = db.planv5.selections[id] ?? [];
      const toolCount =
        7 +
        db.planv5.connections
          .filter((item) => selected.includes(item.value.id))
          .reduce((sum, item) => sum + item.value.tool_count, 0);
      const estimated = input.content.length + toolCount * 150;
      const usable =
        model.contextWindowTokens -
        model.maxOutputTokens -
        Math.max(4096, Math.ceil(model.contextWindowTokens * 0.05));
      return {
        ready: estimated <= usable,
        model_id: model.id,
        tool_count: toolCount,
        estimated_tokens: estimated,
        usable_tokens: usable,
        tool_schema_tokens: toolCount * 150,
        snapshot_hash: "mock-snapshot",
        sandbox_status: "ready",
      };
    },
    async updateConnections(id, ids) {
      const c = ctx.mustFind(
        db.conversations,
        (item) => item.id === id && item.enterpriseId === ctx.enterpriseId(),
        "conversation",
      );
      for (const id of ids) {
        if (
          !db.planv5.connections.some(
            (item) =>
              item.enterpriseId === ctx.enterpriseId() &&
              item.value.id === id &&
              item.value.status === "enabled" &&
              item.value.member_ids.includes(ctx.actor().id),
          )
        )
          throw new Error("MCP_CONNECTION_FORBIDDEN");
      }
      c.selectedMCPConnectionIds = [...ids];
      db.planv5.selections[id] = [...ids];
      c.version = (c.version ?? 1) + 1;
      ctx.save();
      return publicConversation(c);
    },
    async list(query) {
      await ctx.pause();
      const items = db.conversations
        .filter(
          (entry) =>
            entry.enterpriseId === ctx.enterpriseId() &&
            entry.status !== "deleted",
        )
        .sort((a, b) => b.lastMessageAt.localeCompare(a.lastMessageAt));
      return ctx.paginate(items.map(publicConversation), query);
    },
    async get(id) {
      await ctx.pause();
      return publicConversation(
        ctx.mustFind(
          db.conversations,
          (entry) => entry.id === id,
          "conversation",
        ),
      );
    },
    async create(input) {
      await ctx.pause();
      const selectedModelId =
        input?.selected_model_id ??
        db.models.find(
          (model) => model.enterpriseId === ctx.enterpriseId() && model.enabled,
        )?.id;
      if (!selectedModelId) throw new Error("no available model");
      const conversation = {
        id: nextId(db, "conv"),
        enterpriseId: ctx.enterpriseId(),
        title: input?.title ?? "新的会话",
        createdBy: ctx.actor().id,
        selectedModelId,
        status: "active" as const,
        lastMessageAt: ctx.nowIso(),
        createdAt: ctx.nowIso(),
        version: 1,
      };
      db.conversations.unshift(conversation);
      ctx.save();
      return publicConversation(conversation);
    },
    async archive(id) {
      await ctx.pause();
      const conversation = ctx.mustFind(
        db.conversations,
        (entry) => entry.id === id,
        "conversation",
      );
      conversation.status = "archived";
      conversation.version = (conversation.version ?? 1) + 1;
      ctx.save();
      return publicConversation(conversation);
    },
    async listEvents(conversationId) {
      await ctx.pause();
      return db.messages
        .filter((entry) => entry.conversationId === conversationId)
        .map((message, index) =>
          conversationEvent(ctx.enterpriseId(), message, index + 1),
        );
    },
    async updateModel(id, modelId) {
      await ctx.pause();
      const conversation = ctx.mustFind(
        db.conversations,
        (entry) => entry.id === id,
        "conversation",
      );
      const model = ctx.mustFind(
        db.models,
        (entry) =>
          entry.id === modelId && entry.enterpriseId === ctx.enterpriseId(),
        "AI model",
      );
      if (!model.enabled || model.healthStatus !== "healthy") {
        throw new Error("model unavailable");
      }
      conversation.selectedModelId = model.id;
      conversation.version = (conversation.version ?? 1) + 1;
      ctx.save();
      return publicConversation(conversation);
    },
    sendMessage(conversationId, input, options) {
      const dashboardContext = prepareDashboardContext(
        ctx,
        conversationId,
        input,
      );
      const conversation = ctx.mustFind(
        db.conversations,
        (entry) => entry.id === conversationId,
        "conversation",
      );
      const enterpriseUser = db.enterpriseUsers.find(
        (entry) => entry.userId === ctx.actor().id,
      );
      if (!enterpriseUser)
        throw new Error("enterprise enterpriseUser required");
      const month = ctx.nowIso().slice(0, 7);
      const points = db.usagePoints.filter(
        (point) =>
          point.modelId === conversation.selectedModelId &&
          point.date.startsWith(month),
      );
      const departmentUsed = points
        .filter((point) => point.departmentId === enterpriseUser.departmentId)
        .reduce((sum, point) => sum + point.amount, 0);
      const userUsed = points
        .filter((point) => point.userId === enterpriseUser.userId)
        .reduce((sum, point) => sum + point.amount, 0);
      const departmentQuota = db.modelQuotas.find(
        (quota) =>
          quota.modelId === conversation.selectedModelId &&
          quota.subjectType === "department" &&
          quota.subjectId === enterpriseUser.departmentId,
      );
      const userQuota = db.modelQuotas.find(
        (quota) =>
          quota.modelId === conversation.selectedModelId &&
          quota.subjectType === "user" &&
          quota.subjectId === enterpriseUser.userId,
      );
      if (departmentQuota && departmentUsed >= departmentQuota.monthlyAmount) {
        throw new Error("department quota exhausted");
      }
      if (userQuota && userUsed >= userQuota.monthlyAmount) {
        throw new Error("user quota exhausted");
      }
      const userMessage: ChatMessage = {
        id: nextId(db, "msg"),
        conversationId,
        role: "user",
        content: input.content,
        dashboardContext,
        createdAt: ctx.nowIso(),
        modelId: conversation.selectedModelId,
      };
      (db.planv5.dashboardContexts ??= {})[conversationId] = dashboardContext;
      db.messages.push(userMessage);
      conversation.lastMessageAt = userMessage.createdAt;
      ctx.save();
      return streamEnvelopes(
        dashboardContext.mode === "none"
          ? ctx.streamReply(conversationId, input.content)
          : mockDashboardReply(ctx, conversationId, dashboardContext.mode),
        options?.signal,
      );
    },
    subscribe(conversationId, listener) {
      return ctx.emitter.on(`chat:${conversationId}`, listener);
    },
  };
}
