import type { ConversationEvent } from "@argus/api-client/contracts";
import { timelineFromEvents } from "./chat-timeline";
import { parseToolDisplay, type ToolDisplay } from "./tool-display";
export type MessageRole = "user" | "assistant" | "system";

export interface ToolCallTrace {
  display?: ToolDisplay;
  callId: string;
  toolName: string;
  source?: string;
  status: "running" | "success" | "failed";
  summary?: string;
  durationMs?: number;
  startedAt: string;
  input?: unknown;
  output?: unknown;
  resultRef?: string;
  completedAt?: string;
}

export interface ChatMessage {
  kind?:
    "message" | "tool" | "presentation" | "file" | "action" | "interaction";
  interactionId?: string;
  sequence?: number;
  runId?: string;
  fileGroup?: string;
  streamEntries?: ChatMessage[];
  isStreaming?: boolean;
  dashboardContext?: import("@argus/api-client").DashboardChatSelection;
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

function dashboardReference(value: unknown): ChatMessage["dashboardContext"] {
  const data = record(value);
  if (
    !data ||
    !["none", "analyze", "create"].includes(String(data.mode)) ||
    !Array.isArray(data.dashboard_ids) ||
    !data.dashboard_ids.every((id) => typeof id === "string")
  )
    return undefined;
  return {
    mode: data.mode as "none" | "analyze" | "create",
    dashboard_ids: data.dashboard_ids,
  };
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
            source: stringValue(item.source),
            status,
            startedAt,
            summary: stringValue(item.summary),
            display: parseToolDisplay(item.display),
            durationMs: numberValue(item.duration_ms),
            ...(item.input !== undefined ? { input: item.input } : {}),
            ...(item.output !== undefined ? { output: item.output } : {}),
          },
        ];
      })
    : [];

  return {
    id: messageId,
    conversationId,
    role,
    content,
    dashboardContext: dashboardReference(source.dashboard_context),
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
  media_type?: string;
  path?: string;
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
                media_type: stringValue(item.media_type),
                path: stringValue(item.path),
              },
            ]
          : [];
      })
    : [];
}

export function chatMessagesFromEvents(
  events: readonly ConversationEvent[],
): ChatMessage[] {
  return timelineFromEvents(events, chatMessageFromPublic);
}
