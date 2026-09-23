import type { ConversationEvent } from "@argus/api-client/contracts";
export type MessageRole = "user" | "assistant" | "system";

export interface ToolCallTrace {
  callId: string;
  toolName: string;
  status: "running" | "success" | "failed";
  summary?: string;
  durationMs?: number;
  startedAt: string;
}

export interface ChatMessage {
  id: string;
  conversationId: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  modelId?: string;
  modelRevision?: number;
  inputPricePerMillionSnapshot?: number;
  outputPricePerMillionSnapshot?: number;
  inputTokens?: number;
  outputTokens?: number;
  toolCalls?: ToolCallTrace[];
  presentations?: string[];
  pendingActionRefs?: string[];
  files?: ChatFile[];
  artifacts?: ChatFile[];
}

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : null;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

export function chatMessageFromPublic(value: unknown): ChatMessage | null {
  const source = record(value);
  const messageId = stringValue(source?.message_id);
  const conversationId = stringValue(source?.conversation_id);
  const role = stringValue(source?.role);
  const content = stringValue(source?.content);
  const createdAt = stringValue(source?.created_at);
  if (
    !source ||
    !messageId ||
    !conversationId ||
    (role !== "user" && role !== "assistant" && role !== "system") ||
    content === undefined ||
    !createdAt
  ) {
    return null;
  }
  const toolCalls = Array.isArray(source.tool_calls)
    ? source.tool_calls.flatMap((entry): ToolCallTrace[] => {
        const item = record(entry);
        const callId = stringValue(item?.call_id);
        const toolName = stringValue(item?.tool_name);
        const status = stringValue(item?.status);
        const startedAt = stringValue(item?.started_at);
        if (
          !item ||
          !callId ||
          !toolName ||
          !startedAt ||
          (status !== "running" && status !== "success" && status !== "failed")
        ) {
          return [];
        }
        return [
          {
            callId,
            toolName,
            status,
            startedAt,
            summary: stringValue(item.summary),
            durationMs: numberValue(item.duration_ms),
          },
        ];
      })
    : [];

  return {
    id: messageId,
    conversationId,
    role,
    content,
    createdAt,
    modelId: stringValue(source.model_id),
    modelRevision: numberValue(source.model_revision),
    inputPricePerMillionSnapshot: numberValue(
      source.input_price_per_million_snapshot,
    ),
    outputPricePerMillionSnapshot: numberValue(
      source.output_price_per_million_snapshot,
    ),
    inputTokens: numberValue(source.input_tokens),
    outputTokens: numberValue(source.output_tokens),
    toolCalls,
    presentations: stringArray(source.presentations),
    pendingActionRefs: stringArray(source.pending_action_refs),
    files: fileArray(source.files),
    artifacts: fileArray(source.artifacts),
  };
}

export type ChatFile = {
  id: string;
  name: string;
  byte_size: number;
  content_hash?: string;
};
function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}
function fileArray(value: unknown): ChatFile[] {
  return Array.isArray(value)
    ? value.flatMap((entry) => {
        const item = record(entry);
        return item &&
          typeof item.id === "string" &&
          typeof item.name === "string"
          ? [
              {
                id: item.id,
                name: item.name,
                byte_size: Number(item.byte_size) || 0,
                content_hash: stringValue(item.content_hash),
              },
            ]
          : [];
      })
    : [];
}

export function chatMessagesFromEvents(
  events: readonly ConversationEvent[],
): ChatMessage[] {
  const messages: ChatMessage[] = [];
  const assistants = new Map<string, ChatMessage>();
  for (const event of [...events].sort((a, b) => a.sequence - b.sequence)) {
    const payload = record(event.payload) ?? {};
    const explicit = chatMessageFromPublic(payload.message);
    if (explicit) {
      messages.push(explicit);
      continue;
    }
    if (event.event_type === "user_message") {
      messages.push({
        id: event.event_id,
        conversationId: event.conversation_id,
        role: "user",
        content: stringValue(payload.content) ?? "",
        createdAt: event.occurred_at,
        files: fileArray(payload.files),
      });
      continue;
    }
    if (!event.run_id) {
      if (event.event_type === "workspace_file_added")
        messages.push({
          id: event.event_id,
          conversationId: event.conversation_id,
          role: "user",
          content: "",
          createdAt: event.occurred_at,
          files: fileArray([payload.file]),
        });
      continue;
    }
    if (
      ![
        "assistant_message",
        "tool_call_result",
        "tool_presentation",
        "pending_action_created",
        "artifact_published",
      ].includes(event.event_type)
    )
      continue;
    let message = assistants.get(event.run_id);
    if (!message) {
      message = {
        id: `run-${event.run_id}`,
        conversationId: event.conversation_id,
        role: "assistant",
        content: "",
        createdAt: event.occurred_at,
        toolCalls: [],
        presentations: [],
        pendingActionRefs: [],
        artifacts: [],
      };
      assistants.set(event.run_id, message);
      messages.push(message);
    }
    if (event.event_type === "assistant_message") {
      const content = stringValue(payload.content);
      if (content)
        message.content += [message.content ? "\n\n" : "", content].join("");
    }
    if (
      event.event_type === "tool_call_result" &&
      typeof payload.tool_call_id === "string"
    )
      message.toolCalls?.push({
        callId: payload.tool_call_id,
        toolName:
          stringValue(payload.target_tool_id) ??
          stringValue(payload.tool_id) ??
          "tool",
        status: payload.status === "succeeded" ? "success" : "failed",
        summary:
          payload.status === "result_unknown" ? "result_unknown" : undefined,
        startedAt: event.occurred_at,
      });
    if (
      event.event_type === "tool_presentation" &&
      typeof payload.tool_call_id === "string"
    )
      message.presentations?.push(payload.tool_call_id);
    if (
      event.event_type === "pending_action_created" &&
      typeof payload.action_ref === "string"
    )
      message.pendingActionRefs?.push(payload.action_ref);
    if (event.event_type === "artifact_published")
      message.artifacts?.push(...fileArray([payload.artifact]));
  }
  return messages;
}
