import type { ConversationEvent } from "@argus/api-client/contracts";
import type { ChatFile, ChatMessage, ToolCallTrace } from "./chat-view-model";
import { hasChatContent } from "./chat-content";
import { parseToolDisplay } from "./tool-display";
import {
  appendFileEntry,
  fileEntryFromEvent,
  fileFromPublic as file,
} from "./file-timeline";

const object = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const string = (value: unknown) =>
  typeof value === "string" ? value : undefined;
function parse(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

/** Ledger sequence is the source of presentation order; technical events are not messages. */
export function timelineFromEvents(
  events: readonly ConversationEvent[],
  parseMessage: (value: unknown) => ChatMessage | null,
): ChatMessage[] {
  const result: ChatMessage[] = [];
  const seen = new Set<string>();
  const presented = new Set<string>();
  const calls = new Map<
    string,
    { name: string; input: unknown; startedAt: string }
  >();
  const toolEntries = new Map<string, ChatMessage>();
  for (const event of [...events].sort((a, b) => a.sequence - b.sequence)) {
    if (seen.has(event.event_id)) continue;
    seen.add(event.event_id);
    const payload = object(event.payload);
    if (
      event.event_type === "tool_presentation" &&
      typeof payload.result_key === "string" &&
      typeof payload.tool_call_id === "string"
    ) {
      const call = toolEntries.get(payload.tool_call_id)?.toolCalls?.[0];
      if (call)
        call.display = {
          ...(call.display ?? { mode: "template", facts: [], issues: [] }),
          resultKey: payload.result_key,
        };
    }
    const base: ChatMessage = {
      id: event.event_id,
      conversationId: event.conversation_id,
      role: "assistant",
      content: "",
      createdAt: event.occurred_at,
      sequence: event.sequence,
      runId: event.run_id,
    };
    const explicit = parseMessage(payload.message);
    if (explicit) {
      explicit.presentations = explicit.presentations?.filter((id) => {
        if (presented.has(id)) return false;
        presented.add(id);
        return true;
      });
      if (hasChatContent(explicit))
        result.push({ ...base, ...explicit, kind: "message" });
      continue;
    }
    if (event.event_type === "assistant_message") {
      for (const raw of Array.isArray(payload.tool_calls)
        ? payload.tool_calls
        : []) {
        const call = object(raw);
        if (typeof call.id === "string")
          calls.set(call.id, {
            name: string(call.name) ?? "tool",
            input: parse(call.arguments),
            startedAt: event.occurred_at,
          });
      }
      const content = string(payload.content) ?? "";
      if (content.trim()) result.push({ ...base, kind: "message", content });
    } else if (event.event_type === "user_message") {
      const content = string(payload.content) ?? "";
      const references = object(payload.dashboard_context);
      const files = (Array.isArray(payload.files) ? payload.files : [])
        .map(file)
        .filter((item): item is ChatFile => !!item);
      if (content.trim() || files.length)
        result.push({
          ...base,
          kind: "message",
          role: "user",
          content,
          files,
          dashboardContext:
            ["none", "analyze", "create"].includes(String(references.mode)) &&
            Array.isArray(references.dashboard_ids)
              ? (references as unknown as ChatMessage["dashboardContext"])
              : undefined,
        });
    } else if (
      event.event_type === "tool_call_result" &&
      typeof payload.tool_call_id === "string"
    ) {
      const id = payload.tool_call_id;
      const requested = calls.get(id);
      const input = object(requested?.input);
      const trace: ToolCallTrace = {
        callId: id,
        display: parseToolDisplay(payload.display),
        source: string(payload.source),
        toolName:
          string(payload.target_tool_id) ??
          string(payload.tool_id) ??
          requested?.name ??
          "tool",
        status:
          payload.status === "succeeded"
            ? "success"
            : payload.status === "running"
              ? "running"
              : "failed",
        startedAt: requested?.startedAt ?? event.occurred_at,
        completedAt: event.occurred_at,
        durationMs:
          typeof payload.duration_ms === "number"
            ? payload.duration_ms
            : undefined,
        input:
          requested?.name === "tool.invoke"
            ? (input.arguments ?? input)
            : requested?.input,
        output: object(payload.projection).summary,
        resultRef: string(payload.result_ref),
        summary:
          payload.status === "result_unknown" ? "result_unknown" : undefined,
      };
      const entry = {
        ...base,
        id: `tool-${id}`,
        kind: "tool" as const,
        toolCalls: [trace],
      };
      toolEntries.set(id, entry);
      result.push(entry);
    } else if (
      event.event_type === "tool_presentation" &&
      typeof payload.tool_call_id === "string" &&
      !presented.has(payload.tool_call_id)
    ) {
      presented.add(payload.tool_call_id);
      result.push({
        ...base,
        kind: "presentation",
        presentations: [payload.tool_call_id],
        toolCalls: toolEntries.get(payload.tool_call_id)?.toolCalls,
      });
    } else if (
      event.event_type === "pending_action_created" &&
      typeof payload.action_ref === "string"
    ) {
      result.push({
        ...base,
        kind: "action",
        pendingActionRefs: [payload.action_ref],
      });
    } else if (
      event.event_type === "workspace_file_added" ||
      event.event_type === "artifact_published"
    ) {
      const entry = fileEntryFromEvent(event);
      if (entry) appendFileEntry(result, entry);
    }
  }
  return result;
}
