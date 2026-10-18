import type {
  AgentEvent,
  ConversationEvent,
  Run,
  RunCheckpoint,
  StreamEventEnvelope,
} from "@argus/api-client/contracts";
import {
  chatMessageFromPublic,
  type ChatMessage,
  type ToolCallTrace,
} from "./chat-view-model";
import { parseToolEventDisplay } from "./tool-display";
import { projectLiveTimeline } from "./live-timeline";

export type ChatStopReason =
  "completed" | "cancelled" | "failed" | "output_limit";

export type CompactionView = {
  status: "idle" | "running" | "completed" | "failed";
  tokens_before?: number;
  tokens_after?: number;
};

export type ConversationProjection = {
  last_sequence: number;
  event_ids: ReadonlySet<string>;
  events: readonly ConversationEvent[];
  run?: Run;
  checkpoint?: RunCheckpoint;
  message_text: string;
  live_entries: readonly ChatMessage[];
  message_id?: string;
  tool_calls: ReadonlyMap<string, ToolCallTrace>;
  requested_calls: ReadonlyMap<
    string,
    { name: string; input: unknown; requestedAt: string }
  >;
  presentations: readonly string[];
  completed_message?: ChatMessage;
  pending_action_refs: readonly string[];
  execution_refs: readonly string[];
  compaction: CompactionView;
  stop_reason?: ChatStopReason;
};

export const initialConversationProjection: ConversationProjection = {
  last_sequence: 0,
  event_ids: new Set(),
  events: [],
  message_text: "",
  live_entries: [],
  tool_calls: new Map(),
  requested_calls: new Map(),
  presentations: [],
  pending_action_refs: [],
  execution_refs: [],
  compaction: { status: "idle" },
};

function objectPayload(payload: unknown): Record<string, unknown> {
  return typeof payload === "object" && payload !== null
    ? (payload as Record<string, unknown>)
    : {};
}

function stringField(
  payload: Record<string, unknown>,
  key: string,
): string | undefined {
  return typeof payload[key] === "string" ? payload[key] : undefined;
}

export function reduceAgentEvent(
  state: ConversationProjection,
  event: AgentEvent,
): ConversationProjection {
  if (
    state.event_ids.has(event.event_id) ||
    event.sequence <= state.last_sequence
  )
    return state;
  const payload = objectPayload(event.payload);
  const event_ids = new Set(state.event_ids).add(event.event_id);
  const tool_calls = new Map(state.tool_calls);
  const requested_calls = new Map(state.requested_calls);
  let message_text = state.message_text;
  let message_id = state.message_id;
  let presentations = state.presentations;
  let completed_message = state.completed_message;
  let pending_action_refs = state.pending_action_refs;
  let execution_refs = state.execution_refs;
  let compaction = state.compaction;
  let stop_reason = state.stop_reason;

  if (event.event_type === "message_started") {
    message_id = stringField(payload, "message_id") ?? event.message_id;
    message_text = "";
    completed_message = undefined;
  } else if (event.event_type === "message_delta") {
    if (completed_message) {
      message_text = "";
      completed_message = undefined;
    }
    message_text += stringField(payload, "delta") ?? "";
  } else if (event.event_type === "tool_call_started") {
    const ref = stringField(payload, "tool_call_id");
    if (ref) {
      tool_calls.set(ref, {
        callId: ref,
        toolName: stringField(payload, "tool_name") ?? "tool",
        source: stringField(payload, "source"),
        status: "running",
        startedAt: stringField(payload, "started_at") ?? event.occurred_at,
        input: payload.arguments ?? payload.input,
      });
    }
  } else if (event.event_type === "tool_call_completed") {
    const ref = stringField(payload, "tool_call_id");
    const current = ref ? tool_calls.get(ref) : undefined;
    const requested = ref ? requested_calls.get(ref) : undefined;
    if (ref) {
      tool_calls.set(ref, {
        callId: ref,
        toolName:
          stringField(payload, "target_tool_id") ??
          current?.toolName ??
          stringField(payload, "tool_id") ??
          "tool",
        source: stringField(payload, "source") ?? current?.source,
        startedAt:
          current?.startedAt ?? requested?.requestedAt ?? event.occurred_at,
        status:
          payload.status === "succeeded" || payload.status === "success"
            ? "success"
            : "failed",
        summary: stringField(payload, "summary"),
        display: parseToolEventDisplay(payload),
        durationMs:
          typeof payload.duration_ms === "number"
            ? payload.duration_ms
            : undefined,
        input: current?.input ?? requested?.input,
        output:
          payload.projection && typeof payload.projection === "object"
            ? (payload.projection as Record<string, unknown>).summary
            : payload.output,
        resultRef: stringField(payload, "result_ref"),
        completedAt: event.occurred_at,
      });
    }
  } else if (event.event_type === "pending_action_created") {
    const ref = stringField(payload, "action_ref");
    if (ref && !pending_action_refs.includes(ref))
      pending_action_refs = [...pending_action_refs, ref];
  } else if (event.event_type === "context_compaction_started") {
    compaction = {
      status: "running",
      tokens_before: Number(payload.tokens_before) || undefined,
    };
  } else if (event.event_type === "context_compaction_completed") {
    compaction = {
      status: "completed",
      tokens_before: Number(payload.tokens_before) || compaction.tokens_before,
      tokens_after: Number(payload.tokens_after) || undefined,
    };
  } else if (event.event_type === "context_compaction_failed") {
    compaction = { ...compaction, status: "failed" };
  } else if (event.event_type === "run_completed") {
    stop_reason = [
      "user_cancelled",
      "pending_action_cancelled",
      "request_cancelled",
      "workspace_deleted",
      "conversation_deleted",
    ].includes(stringField(payload, "stop_reason") ?? "")
      ? "cancelled"
      : payload.stop_reason === "output_limit"
        ? "output_limit"
        : "completed";
  } else if (event.event_type === "run_failed") {
    stop_reason = "failed";
  }
  if (event.event_type === "tool_presentation") {
    const ref = stringField(payload, "tool_call_id");
    const key = stringField(payload, "result_key");
    const call = ref ? tool_calls.get(ref) : undefined;
    if (ref && key && call)
      tool_calls.set(ref, {
        ...call,
        display: {
          ...(call.display ?? { mode: "template", facts: [], issues: [] }),
          resultKey: key,
        },
      });
    if (ref && !presentations.includes(ref))
      presentations = [...presentations, ref];
  }
  if (event.event_type === "message_completed") {
    completed_message = chatMessageFromPublic(payload.message) ?? undefined;
    message_text = completed_message?.content ?? "";
    for (const raw of Array.isArray(payload.tool_calls)
      ? payload.tool_calls
      : []) {
      const call = objectPayload(raw);
      if (typeof call.id !== "string") continue;
      let input: unknown = call.arguments;
      if (typeof input === "string") {
        try {
          input = JSON.parse(input);
        } catch {
          /* Retain an incomplete argument string as evidence. */
        }
      }
      requested_calls.set(call.id, {
        name: String(call.name ?? "tool"),
        input:
          call.name === "tool.invoke"
            ? (objectPayload(input).arguments ?? input)
            : input,
        requestedAt: event.occurred_at,
      });
    }
  }
  const execution_ref = stringField(payload, "execution_ref");
  if (execution_ref && !execution_refs.includes(execution_ref))
    execution_refs = [...execution_refs, execution_ref];

  return {
    ...state,
    last_sequence: event.sequence,
    event_ids,
    message_text,
    message_id,
    live_entries: projectLiveTimeline(
      state.live_entries,
      event,
      message_text,
      tool_calls,
      completed_message,
    ),
    tool_calls,
    requested_calls,
    presentations,
    completed_message,
    pending_action_refs,
    execution_refs,
    compaction,
    stop_reason,
  };
}

export function reduceConversationEvent(
  state: ConversationProjection,
  event: ConversationEvent,
): ConversationProjection {
  if (
    state.event_ids.has(event.event_id) ||
    event.sequence <= state.last_sequence
  )
    return state;
  return {
    ...state,
    last_sequence: event.sequence,
    event_ids: new Set(state.event_ids).add(event.event_id),
    events: [...state.events, event],
  };
}

export function reduceStreamEnvelope(
  state: ConversationProjection,
  envelope: StreamEventEnvelope,
): ConversationProjection {
  if (envelope.event_type !== "agent_event") return state;
  return reduceAgentEvent(state, envelope.data as AgentEvent);
}
