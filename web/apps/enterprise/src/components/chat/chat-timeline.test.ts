import { describe, expect, it } from "vitest";
import type {
  ConversationEvent,
  AgentEvent,
} from "@argus/api-client/contracts";
import { chatMessagesFromEvents, type ChatMessage } from "./chat-view-model";
import {
  initialConversationProjection,
  reduceAgentEvent,
} from "./event-reducer";
import { mergeTimeline } from "./live-timeline";

const when = (sequence: number) =>
  `2026-10-12T13:40:${String(sequence).padStart(2, "0")}Z`;
function event(
  sequence: number,
  event_type: string,
  payload: Record<string, unknown>,
): ConversationEvent {
  return {
    schema_version: "argus.conversation_event/v1",
    event_id: `e${sequence}`,
    sequence,
    conversation_id: "conversation",
    run_id: "run",
    event_type,
    occurred_at: when(sequence),
    payload,
  } as ConversationEvent;
}
describe("conversation timeline", () => {
  it("updates existing cards for persisted ready events while preserving independent corrected queries", () => {
    const sources = [
      event(1, "tool_presentation", {
        tool_call_id: "original",
        status: "fetching",
      }),
      event(2, "tool_presentation", {
        tool_call_id: "corrected",
        status: "fetching",
      }),
      event(3, "tool_presentation", {
        tool_call_id: "original",
        status: "data_ready",
      }),
      event(4, "tool_presentation", {
        tool_call_id: "corrected",
        status: "data_ready",
      }),
      event(5, "tool_presentation", {
        tool_call_id: "original",
        status: "data_ready",
      }),
    ];
    const history = chatMessagesFromEvents(sources);
    expect(history.map((entry) => entry.presentations)).toEqual([
      ["original"],
      ["corrected"],
    ]);
    expect(history.map((entry) => entry.sequence)).toEqual([1, 2]);
    let projection = initialConversationProjection;
    for (const source of sources)
      projection = reduceAgentEvent(
        projection,
        source as unknown as AgentEvent,
      );
    expect(projection.live_entries.map((entry) => entry.presentations)).toEqual(
      history.map((entry) => entry.presentations),
    );
    expect(
      mergeTimeline(history, {
        id: "stream",
        conversationId: "conversation",
        role: "assistant",
        content: "",
        createdAt: when(1),
        streamEntries: [...projection.live_entries],
      }),
    ).toHaveLength(2);
  });

  it("does not duplicate a card embedded in a completed message when its ready event is replayed", () => {
    const history = chatMessagesFromEvents([
      event(1, "assistant_message", {
        message: {
          message_id: "message",
          conversation_id: "conversation",
          role: "assistant",
          content: "完成查询",
          created_at: when(1),
          presentations: ["query"],
        },
      }),
      event(2, "tool_presentation", {
        tool_call_id: "query",
        status: "data_ready",
      }),
    ]);
    expect(history).toHaveLength(1);
    expect(history[0]?.content).toBe("完成查询");
    expect(history[0]?.presentations).toEqual(["query"]);
  });
  it("preserves a result presentation without prose after reloading history", () => {
    const timeline = chatMessagesFromEvents([
      event(1, "assistant_message", {
        message: {
          message_id: "query-result",
          conversation_id: "conversation",
          role: "assistant",
          content: "",
          created_at: when(1),
          presentations: ["query-call"],
        },
      }),
      event(2, "assistant_message", {
        message: {
          message_id: "empty",
          conversation_id: "conversation",
          role: "assistant",
          content: "",
          created_at: when(2),
        },
      }),
    ]);
    expect(timeline).toHaveLength(1);
    expect(timeline[0]?.presentations).toEqual(["query-call"]);
  });
  it("interleaves actual tool results and cards between assistant segments and excludes technical empty replies", () => {
    const events = [
      event(1, "user_message", { content: "主机是否告警？" }),
      event(2, "assistant_message", {
        content: "先检查当前事件。",
        tool_calls: [
          {
            id: "call",
            name: "tool.invoke",
            arguments: JSON.stringify({
              tool_id: "alert.event.query",
              arguments: { resource_ids: ["host"] },
            }),
          },
        ],
      }),
      event(3, "tool_call_result", {
        tool_call_id: "call",
        tool_id: "tool.invoke",
        target_tool_id: "alert.event.query",
        status: "succeeded",
        projection: { summary: { page: { items: [{ lifecycle: "firing" }] } } },
      }),
      event(4, "tool_presentation", { tool_call_id: "call" }),
      event(5, "assistant_message", { content: "**存在一条演示告警。**" }),
      event(6, "workspace_file_added", {
        file: { id: "report", name: "诊断.md", byte_size: 80 },
      }),
      event(7, "run_state_changed", { status: "succeeded" }),
    ];
    const timeline = chatMessagesFromEvents(
      [...events].reverse().concat(events[3]!),
    );
    expect(timeline.map((item) => item.kind)).toEqual([
      "message",
      "message",
      "tool",
      "presentation",
      "message",
      "file",
    ]);
    expect(timeline.filter((item) => item.role === "user")).toHaveLength(1);
    expect(timeline[2]?.toolCalls?.[0]).toMatchObject({
      toolName: "alert.event.query",
      input: { resource_ids: ["host"] },
      output: { page: { items: [{ lifecycle: "firing" }] } },
      status: "success",
    });
  });
  it("groups generated query files without inventing a user message", () => {
    const timeline = chatMessagesFromEvents(
      [1, 2, 3].map((sequence) =>
        event(sequence, "workspace_file_added", {
          file: {
            id: `file${sequence}`,
            name: `query${sequence}.json`,
            path: `/workspace/queries/job/attempt/query${sequence}.json`,
          },
        }),
      ),
    );
    expect(timeline).toHaveLength(1);
    expect(timeline[0]).toMatchObject({
      role: "system",
      kind: "file",
      fileGroup: "job",
    });
    expect(timeline[0]?.files).toHaveLength(3);
  });
  it("preserves streamed segments across real completed-message envelopes without message_started", () => {
    let projection = initialConversationProjection;
    const accept = (
      sequence: number,
      type: string,
      payload: Record<string, unknown>,
    ) => {
      projection = reduceAgentEvent(
        projection,
        event(sequence, type, payload) as unknown as AgentEvent,
      );
    };
    accept(1, "message_delta", { delta: "正在查询。" });
    accept(2, "message_completed", {
      message: {
        message_id: "a",
        conversation_id: "conversation",
        role: "assistant",
        content: "正在查询。",
        created_at: when(1),
      },
      tool_calls: [
        {
          id: "call",
          name: "tool.invoke",
          arguments: JSON.stringify({
            tool_id: "telemetry.promql.query",
            arguments: { query: "cpu", resource_ids: ["host"] },
          }),
        },
      ],
    });
    accept(3, "tool_call_completed", {
      tool_call_id: "call",
      target_tool_id: "telemetry.promql.query",
      status: "succeeded",
      projection: { summary: { value: 3.5 } },
    });
    accept(4, "tool_presentation", { tool_call_id: "call" });
    accept(5, "message_delta", { delta: "CPU 为 **3.5%**。" });
    expect(projection.message_text).toBe("CPU 为 **3.5%**。");
    expect(projection.tool_calls.get("call")?.input).toEqual({
      query: "cpu",
      resource_ids: ["host"],
    });
    expect(projection.live_entries.map((item) => item.kind)).toEqual([
      "message",
      "tool",
      "presentation",
      "message",
    ]);
    expect(projection.live_entries[0]?.content).toBe("正在查询。");
    const history: ChatMessage[] = [
      { ...projection.live_entries[0]!, conversationId: "conversation" },
    ];
    const merged = mergeTimeline(history, {
      id: "stream",
      conversationId: "conversation",
      role: "assistant",
      content: projection.message_text,
      createdAt: when(5),
      streamEntries: [...projection.live_entries],
    });
    expect(merged).toHaveLength(4);
    expect(merged[3]?.content).toBe("CPU 为 **3.5%**。");
  });
  it("does not replay previous text for a tool-only completed message with null content", () => {
    let projection = reduceAgentEvent(
      initialConversationProjection,
      event(1, "message_completed", {
        message: {
          message_id: "a",
          conversation_id: "conversation",
          role: "assistant",
          content: "先查询指标。",
          created_at: when(1),
        },
      }) as unknown as AgentEvent,
    );
    projection = reduceAgentEvent(
      projection,
      event(2, "message_completed", {
        message: {
          message_id: "b",
          conversation_id: "conversation",
          role: "assistant",
          content: null,
          created_at: when(2),
        },
      }) as unknown as AgentEvent,
    );
    expect(projection.message_text).toBe("");
    expect(projection.live_entries).toHaveLength(1);
    expect(projection.live_entries[0]?.content).toBe("先查询指标。");
  });
});
