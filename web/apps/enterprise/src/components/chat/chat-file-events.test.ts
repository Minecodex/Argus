import { describe, expect, it } from "vitest";
import type {
  AgentEvent,
  ConversationEvent,
} from "@argus/api-client/contracts";
import { chatMessagesFromEvents, type ChatMessage } from "./chat-view-model";
import {
  initialConversationProjection,
  reduceAgentEvent,
} from "./event-reducer";
import { mergeTimeline } from "./live-timeline";

function event(
  sequence: number,
  event_type: string,
  payload: Record<string, unknown>,
  run_id?: string,
): ConversationEvent {
  return {
    schema_version: "argus.conversation_event/v1",
    event_id: `event-${sequence}`,
    sequence,
    conversation_id: "conversation",
    enterprise_id: "enterprise",
    actor_type: "service",
    content_hash: "0".repeat(64),
    data_classification: "internal",
    event_type,
    occurred_at: `2026-10-12T01:00:${String(sequence).padStart(2, "0")}Z`,
    payload,
    run_id,
  } as ConversationEvent;
}

const file = (id: string, group = "query") => ({
  id,
  name: `${id}.json`,
  byte_size: 30,
  path: `/workspace/queries/${group}/attempt/${id}.json`,
});
const stream = (streamEntries: ChatMessage[]): ChatMessage => ({
  id: "stream",
  conversationId: "conversation",
  role: "assistant",
  content: "",
  createdAt: "2026-10-12T01:00:00Z",
  streamEntries,
});
function agent(source: ConversationEvent): AgentEvent {
  return { ...source, schema_version: "argus.agent_event/v1" } as AgentEvent;
}

describe("file events in chat", () => {
  it("keeps the platform metadata index out of business file activities", () => {
    const entries = chatMessagesFromEvents([
      event(1, "workspace_file_added", {
        file: { ...file("meta"), path: "/workspace/results/meta.json" },
        metadata_index: true,
      }),
      event(2, "workspace_file_added", {
        file: {
          ...file("status"),
          path: "/workspace/results/nginx/status.json",
        },
      }),
    ]);
    expect(
      entries.flatMap((entry) => entry.files ?? []).map((item) => item.id),
    ).toEqual(["status"]);
  });
  it.each([undefined, "run"])(
    "shows generated files with run_id=%s as file activities without user replies",
    (runId) => {
      const history = chatMessagesFromEvents([
        event(1, "workspace_file_added", { file: file("cpu") }, runId),
        event(2, "workspace_file_added", { file: file("memory") }, runId),
        event(3, "workspace_file_added", { file: file("manifest") }, runId),
        event(
          4,
          "assistant_message",
          { content: "  ", tool_calls: [{ id: "call", name: "read" }] },
          "run",
        ),
        event(5, "assistant_message", { content: "分析完成" }, "run"),
      ]);
      expect(history.map((entry) => entry.kind)).toEqual(["file", "message"]);
      expect(history[0]).toMatchObject({
        role: "system",
        content: "",
        fileGroup: "query",
      });
      expect(history[0]?.files?.map((item) => item.id)).toEqual([
        "cpu",
        "memory",
        "manifest",
      ]);
      expect(history.some((entry) => entry.role === "user")).toBe(false);
    },
  );

  it("retains files and immutable deliveries while streaming without empty message placeholders", () => {
    const sources = [
      event(1, "message_started", { message_id: "empty" }, "run"),
      event(
        2,
        "message_completed",
        {
          message: {
            message_id: "empty",
            conversation_id: "conversation",
            role: "assistant",
            content: "",
            created_at: "2026-10-12T01:00:01Z",
          },
        },
        "run",
      ),
      event(3, "workspace_file_added", { file: file("cpu") }, "run"),
      event(
        4,
        "artifact_published",
        { artifact: { id: "report", name: "report.md", byte_size: 20 } },
        "run",
      ),
    ];
    const projection = sources.reduce(
      (state, source) => reduceAgentEvent(state, agent(source)),
      initialConversationProjection,
    );
    const merged = mergeTimeline([], stream([...projection.live_entries]));
    expect(merged).toHaveLength(2);
    expect(merged.map((entry) => entry.role)).toEqual(["system", "system"]);
    expect(merged[0]?.files?.[0]?.id).toBe("cpu");
    expect(merged[1]?.artifacts?.[0]?.id).toBe("report");
    expect(
      mergeTimeline(
        chatMessagesFromEvents(sources),
        stream([...projection.live_entries]),
      ),
    ).toHaveLength(2);
  });

  it("merges a partly persisted query group without dropping or duplicating new files", () => {
    const first = event(1, "workspace_file_added", { file: file("cpu") });
    const second = event(2, "workspace_file_added", { file: file("memory") });
    const history = chatMessagesFromEvents([first]);
    const before = structuredClone(history);
    const projection = [first, second].reduce(
      (state, source) => reduceAgentEvent(state, agent(source)),
      initialConversationProjection,
    );
    const live = stream([...projection.live_entries]);
    const merged = mergeTimeline(history, live);
    expect(merged).toHaveLength(1);
    expect(merged[0]?.files?.map((item) => item.id)).toEqual(["cpu", "memory"]);
    expect(history).toEqual(before);
    expect(mergeTimeline(merged, live)).toEqual(merged);
  });

  it("ignores malformed file events and preserves separate queries and published artifacts", () => {
    const history = chatMessagesFromEvents([
      event(1, "workspace_file_added", { file: {} }),
      event(2, "workspace_file_added", { file: file("cpu", "job1") }),
      event(3, "workspace_file_added", { file: file("memory", "job2") }),
      event(4, "artifact_published", {
        artifact: { id: "report", name: "report.md" },
      }),
    ]);
    expect(history).toHaveLength(3);
    expect(history[2]?.artifacts?.[0]?.name).toBe("report.md");
  });
});
