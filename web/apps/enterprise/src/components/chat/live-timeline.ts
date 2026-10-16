import type { AgentEvent } from "@argus/api-client/contracts";
import type { ChatMessage, ToolCallTrace } from "./chat-view-model";
import { hasChatContent } from "./chat-content";
import { appendFileEntry, fileEntryFromEvent } from "./file-timeline";

/** Keep separate assistant segments and activities while the Run is streaming. */
export function projectLiveTimeline(
  previous: readonly ChatMessage[],
  event: AgentEvent,
  text: string,
  calls: ReadonlyMap<string, ToolCallTrace>,
  completed?: ChatMessage,
): ChatMessage[] {
  const entries = [...previous];
  const payload = event.payload as Record<string, unknown>;
  const base: ChatMessage = {
    id: event.event_id,
    conversationId: "",
    role: "assistant",
    content: "",
    runId: event.run_id,
    createdAt: event.occurred_at,
  };
  const callId =
    typeof payload.tool_call_id === "string" ? payload.tool_call_id : undefined;
  if (event.event_type === "tool_presentation" && callId && calls.has(callId)) {
    for (let index = 0; index < entries.length; index++) {
      const entry = entries[index]!;
      if (entry.toolCalls?.some((call) => call.callId === callId))
        entries[index] = {
          ...entry,
          toolCalls: entry.toolCalls.map((call) =>
            call.callId === callId ? calls.get(callId)! : call,
          ),
        };
    }
  }
  if (event.event_type === "message_started") {
    entries.push({
      ...base,
      id: String(payload.message_id ?? event.message_id ?? event.event_id),
      kind: "message",
      isStreaming: true,
    });
  } else if (
    event.event_type === "message_delta" ||
    event.event_type === "message_completed"
  ) {
    const index = entries
      .map((entry) => entry.kind === "message" && entry.isStreaming)
      .lastIndexOf(true);
    const isStreaming = event.event_type !== "message_completed";
    if (index >= 0)
      entries[index] = {
        ...entries[index]!,
        ...(completed ?? {}),
        content: completed?.content ?? text,
        runId: event.run_id,
        isStreaming,
      };
    else if (text.trim() || completed?.content.trim())
      entries.push({
        ...base,
        ...(completed ?? {}),
        kind: "message",
        content: completed?.content ?? text,
        isStreaming,
      });
  } else if (
    (event.event_type === "tool_call_started" ||
      event.event_type === "tool_call_completed") &&
    callId &&
    calls.has(callId)
  ) {
    const index = entries.findIndex(
      (entry) =>
        entry.toolCalls?.[0]?.callId === callId && entry.kind === "tool",
    );
    const entry: ChatMessage = {
      ...base,
      id: `tool-${callId}`,
      kind: "tool",
      toolCalls: [calls.get(callId)!],
    };
    if (index < 0) entries.push(entry);
    else entries[index] = { ...entries[index]!, toolCalls: entry.toolCalls };
  } else if (
    event.event_type === "tool_presentation" &&
    callId &&
    !entries.some((entry) => entry.presentations?.includes(callId))
  ) {
    entries.push({
      ...base,
      kind: "presentation",
      presentations: [callId],
      toolCalls: calls.has(callId) ? [calls.get(callId)!] : [],
    });
  } else if (
    event.event_type === "pending_action_created" &&
    typeof payload.action_ref === "string"
  ) {
    entries.push({
      ...base,
      kind: "action",
      pendingActionRefs: [payload.action_ref],
    });
  } else {
    const entry = fileEntryFromEvent(event);
    if (entry) appendFileEntry(entries, entry);
  }
  return entries;
}

export function mergeTimeline(
  history: ChatMessage[],
  streaming: ChatMessage | null,
): ChatMessage[] {
  if (!streaming) return history;
  const live = streaming.streamEntries ?? [streaming];
  const result = [...history];
  for (const raw of live) {
    const entry = { ...raw, conversationId: streaming.conversationId };
    if (entry.kind === "file") {
      entry.files = entry.files?.filter(
        (file) =>
          !result.some((stored) =>
            stored.files?.some((item) => item.id === file.id),
          ),
      );
      entry.artifacts = entry.artifacts?.filter(
        (file) =>
          !result.some((stored) =>
            stored.artifacts?.some((item) => item.id === file.id),
          ),
      );
      if (!hasChatContent(entry)) continue;
      const grouped = result.findIndex(
        (stored) => stored.kind === "file" && stored.id === entry.id,
      );
      if (grouped >= 0) {
        const stored = result[grouped]!;
        result[grouped] = {
          ...stored,
          files: [...(stored.files ?? []), ...(entry.files ?? [])],
          artifacts: [...(stored.artifacts ?? []), ...(entry.artifacts ?? [])],
        };
        continue;
      }
    }
    if (!hasChatContent(entry)) continue;
    const persisted = history.some(
      (stored) =>
        stored.id === entry.id ||
        (entry.kind === "tool" &&
          stored.kind === "tool" &&
          stored.toolCalls?.some(
            (call) => call.callId === entry.toolCalls?.[0]?.callId,
          )) ||
        (entry.kind === "presentation" &&
          entry.presentations?.some((id) =>
            stored.presentations?.includes(id),
          )) ||
        (entry.kind === "action" &&
          entry.pendingActionRefs?.some((id) =>
            stored.pendingActionRefs?.includes(id),
          )) ||
        ((!entry.kind || entry.kind === "message") &&
          stored.role === "assistant" &&
          (!entry.runId || stored.runId === entry.runId) &&
          !!entry.content.trim() &&
          stored.content.trim() === entry.content.trim()),
    );
    if (persisted) continue;
    const index = result.findIndex(
      (stored) => Date.parse(stored.createdAt) > Date.parse(entry.createdAt),
    );
    if (index < 0) result.push(entry);
    else result.splice(index, 0, entry);
  }
  return result;
}
