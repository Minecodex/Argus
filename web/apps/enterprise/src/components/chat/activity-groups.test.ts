import { expect, it } from "vitest";
import { groupActivitySegments, groupActivityStatus } from "./activity-groups";
import {
  activityKind,
  activityStatus,
  parseActivity,
  settleUnfinishedActivity,
} from "./tool-activity-model";
import {
  chatMessagesFromEvents,
  type ChatMessage,
  type ToolCallTrace,
} from "./chat-view-model";
import {
  initialConversationProjection,
  reduceAgentEvent,
} from "./event-reducer";
import { mergeTimeline } from "./live-timeline";
import { combineToolActivities } from "./tool-activities";
import type {
  ConversationEvent,
  AgentEvent,
} from "@argus/api-client/contracts";

const call = (
  id: string,
  source = "sandbox",
  toolName = "read",
): ToolCallTrace => ({
  callId: id,
  source,
  toolName,
  status: "success",
  startedAt: "2026-11-17T00:00:00Z",
});
const message = (
  id: string,
  extra: Partial<ChatMessage> = {},
): ChatMessage => ({
  id,
  conversationId: "conversation",
  runId: "run",
  role: "assistant",
  kind: "tool",
  content: "",
  createdAt: "2026-11-17T00:00:00Z",
  toolCalls: [call(id)],
  ...extra,
});
const event = (sequence: number, event_type: string, payload: unknown) =>
  ({
    schema_version: "argus.conversation_event/v1",
    event_id: `event-${sequence}`,
    sequence,
    conversation_id: "conversation",
    enterprise_id: "enterprise",
    run_id: "run",
    step_id: "step",
    event_type,
    payload,
    occurred_at: `2026-11-17T00:00:${String(sequence).padStart(2, "0")}Z`,
    actor_type: "service",
    content_hash: "hash",
    data_classification: "internal",
  }) as ConversationEvent;

it("groups internal, direct workspace and external parameter tools together without granting external semantics", () => {
  const external = {
    ...call("external", "external_mcp", "edit"),
    input: { path: "/workspace/pretend" },
  };
  expect(
    groupActivitySegments([
      message("one"),
      message("two", { toolCalls: [external] }),
      message("three", { toolCalls: [call("native", "argus", "tool.invoke")] }),
    ]),
  ).toHaveLength(1);
  expect(activityKind(external)).toBe("tool");
  const evidence = {
    schema: "argus.tool_activity/v1",
    kind: "file_change",
    path: "/workspace/file",
    change: {
      operation: "create",
      complete: true,
      added: 2,
      removed: 0,
      lines: [{ type: "add", content: "actual" }],
    },
  };
  expect(parseActivity(evidence, "external_mcp")).toBeUndefined();
  expect(parseActivity(evidence, "customer_mcp")).toBeUndefined();
  expect(parseActivity(evidence, "sandbox")?.change?.added).toBe(2);
  expect(activityKind(call("legacy", undefined, "write"))).toBe("file_change");
  expect(
    activityKind({ ...call("legacy"), source: undefined, toolName: "write" }),
  ).toBe("tool");
});

it("keeps prose, approvals, interactions, file delivery, conversation and Run boundaries", () => {
  for (const barrier of [
    message("prose", {
      kind: "message",
      content: "Actual model commentary",
      toolCalls: [],
    }),
    message("approval", { pendingActionRefs: ["action"] }),
    message("question", { interactionId: "question" }),
    message("file", {
      kind: "file",
      artifacts: [{ id: "file", name: "report.md", byte_size: 5 }],
      toolCalls: [],
    }),
  ]) {
    expect(
      groupActivitySegments([message("one"), barrier, message("two")]).map(
        (item) => item.kind,
      ),
    ).toEqual(["tools", "message", "tools"]);
  }
  expect(
    groupActivitySegments([
      message("one"),
      message("two", { runId: "other" }),
      message("three", { conversationId: "other" }),
    ]),
  ).toHaveLength(3);
  expect(
    groupActivitySegments([
      message("mixed", { kind: "message", content: "Before tools" }),
      message("next"),
    ]).map((item) => item.kind),
  ).toEqual(["message", "tools"]);
});

it("distinguishes queue, cancellation, unknown and command failure from success", () => {
  expect(
    activityStatus({ ...call("q"), rawStatus: "queued", status: "running" }),
  ).toBe("queued");
  expect(activityStatus({ ...call("u"), rawStatus: "result_unknown" })).toBe(
    "unknown",
  );
  expect(activityStatus({ ...call("c"), rawStatus: "cancelled" })).toBe(
    "cancelled",
  );
  expect(
    activityStatus({
      ...call("bash", "sandbox", "bash"),
      output: { exit_code: 7 },
    }),
  ).toBe("failed");
  expect(
    activityStatus({
      ...call("bash", "external_mcp", "bash"),
      output: { exit_code: 7 },
    }),
  ).toBe("success");
  expect(
    groupActivityStatus([call("ok"), { ...call("bad"), status: "failed" }]),
  ).toBe("failed");
  expect(
    activityStatus(
      settleUnfinishedActivity({
        ...call("queued"),
        status: "running",
        rawStatus: "queued",
      }),
    ),
  ).toBe("cancelled");
  expect(
    activityStatus(
      settleUnfinishedActivity({
        ...call("dispatched"),
        status: "running",
        rawStatus: "running",
      }),
    ),
  ).toBe("unknown");
});

it("anchors calls at request order even when completions arrive out of order, equally in history and live", () => {
  const events = [
    event(1, "assistant_message", {
      content: "Inspecting files",
      tool_calls: [
        { id: "a", name: "read", arguments: '{"path":"/workspace/a"}' },
        { id: "b", name: "read", arguments: '{"path":"/workspace/b"}' },
      ],
      tool_sources: { a: "sandbox", b: "sandbox" },
    }),
    event(2, "tool_call_started", {
      tool_call_id: "b",
      tool_name: "read",
      source: "sandbox",
    }),
    event(3, "tool_call_result", {
      tool_call_id: "b",
      tool_id: "read",
      source: "sandbox",
      status: "succeeded",
      projection: { summary: { content: "B" } },
    }),
    event(4, "tool_call_started", {
      tool_call_id: "a",
      tool_name: "read",
      source: "sandbox",
    }),
    event(5, "tool_call_result", {
      tool_call_id: "a",
      tool_id: "read",
      source: "sandbox",
      status: "cancelled",
      projection: { summary: {} },
    }),
    event(6, "assistant_message", { content: "The result", tool_calls: [] }),
  ];
  const history = chatMessagesFromEvents(events);
  let live = initialConversationProjection;
  for (const entry of events) {
    const payload = entry.payload as Record<string, unknown>;
    live = reduceAgentEvent(live, {
      ...entry,
      event_type:
        entry.event_type === "tool_call_result"
          ? "tool_call_completed"
          : entry.event_type === "assistant_message"
            ? "message_completed"
            : entry.event_type,
      payload:
        entry.event_type === "assistant_message"
          ? {
              ...payload,
              message: {
                message_id: entry.event_id,
                conversation_id: "conversation",
                role: "assistant",
                content: payload.content,
                created_at: entry.occurred_at,
              },
            }
          : payload,
    } as unknown as AgentEvent);
  }
  const shape = (items: readonly ChatMessage[]) =>
    items.map((item) => ({
      content: item.content,
      calls: (item.toolCalls ?? []).map((call) => ({
        id: call.callId,
        state: activityStatus(call),
        input: call.input,
      })),
    }));
  expect(shape(live.live_entries)).toEqual(shape(history));
  expect(
    history.flatMap((item) => item.toolCalls?.map((call) => call.callId) ?? []),
  ).toEqual(["a", "b"]);
  const stale = chatMessagesFromEvents(events.slice(0, 1));
  const merged = mergeTimeline(stale, {
    ...message("stream"),
    streamEntries: [...live.live_entries],
    toolCalls: [],
  });
  expect(
    merged.flatMap(
      (item) => item.toolCalls?.map((call) => activityStatus(call)) ?? [],
    ),
  ).toEqual(["cancelled", "success"]);
});

it("attaches a native template to one call without duplicating its activity", () => {
  const entries = combineToolActivities([
    message("one"),
    message("two"),
    message("presentation", {
      kind: "presentation",
      toolCalls: [call("one")],
      presentations: ["one"],
    }),
  ]);
  expect(groupActivitySegments(entries)).toHaveLength(1);
  expect(entries.flatMap((item) => item.toolCalls ?? [])).toHaveLength(2);
});
