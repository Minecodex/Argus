import { describe, expect, it } from "vitest";
import { combineToolActivities } from "./tool-activities";
import type { ChatMessage } from "./chat-view-model";

const entry = (id: string, patch: Partial<ChatMessage> = {}): ChatMessage => ({
  id,
  conversationId: "conversation",
  role: "assistant",
  content: "",
  createdAt: "2026-10-15T00:00:00Z",
  ...patch,
});
const call = {
  callId: "call",
  toolName: "host.list",
  status: "success" as const,
  startedAt: "2026-10-15T00:00:00Z",
};

describe("tool activity projection", () => {
  it("reuses a sealed result only within the exact result/attempt key", () => {
    const display = {
      mode: "template" as const,
      facts: [],
      issues: [],
      resultKey: "query:one:attempt-a",
    };
    const result = combineToolActivities([
      entry("one", {
        toolCalls: [{ ...call, display }],
        presentations: [call.callId],
      }),
      entry("read", {
        toolCalls: [{ ...call, callId: "read", display }],
        presentations: ["read"],
      }),
      entry("corrected", {
        toolCalls: [
          {
            ...call,
            callId: "new",
            display: { ...display, resultKey: "query:one:attempt-b" },
          },
        ],
        presentations: ["new"],
      }),
    ]);
    expect(result.map((item) => item.presentations)).toEqual([
      [call.callId],
      [],
      ["new"],
    ]);
    expect(result[1]?.toolCalls?.[0]?.callId).toBe("read");
  });
  it("puts preview details and repeated status reads into one authoritative action card", () => {
    const display = {
      mode: "action" as const,
      actionRef: "action-1",
      facts: [],
      issues: [],
    };
    const timeline = [
      entry("preview", {
        kind: "tool",
        toolCalls: [{ ...call, display }],
        presentations: [call.callId],
      }),
      entry("action", { kind: "action", pendingActionRefs: ["action-1"] }),
      entry("read", {
        kind: "tool",
        toolCalls: [{ ...call, callId: "status", display }],
      }),
      entry("second-action", {
        kind: "action",
        pendingActionRefs: ["action-1"],
      }),
    ];
    const before = JSON.stringify(timeline);
    const result = combineToolActivities(timeline);
    expect(result).toHaveLength(1);
    expect(result[0]?.pendingActionRefs).toEqual(["action-1"]);
    expect(result[0]?.presentations).toEqual([call.callId]);
    expect(result[0]?.toolCalls).toHaveLength(2);
    expect(JSON.stringify(timeline)).toBe(before);
  });
  it("joins a later result to its call and preserves prose and confirmation order", () => {
    const original = [
      entry("call", { kind: "tool", toolCalls: [call] }),
      entry("prose", { content: "Found the host" }),
      entry("view", {
        kind: "presentation",
        toolCalls: [call],
        presentations: ["call"],
      }),
      entry("approval", { kind: "action", pendingActionRefs: ["action"] }),
    ];
    const before = JSON.stringify(original);
    const projected = combineToolActivities(original);
    expect(projected.map((item) => item.id)).toEqual([
      "call",
      "prose",
      "approval",
    ]);
    expect(projected[0]?.presentations).toEqual(["call"]);
    expect(JSON.stringify(original)).toBe(before);
    expect(combineToolActivities(projected)).toEqual(projected);
  });

  it("keeps standalone results and does not attach across conversations", () => {
    const projected = combineToolActivities([
      entry("call", { kind: "tool", toolCalls: [call] }),
      entry("foreign", {
        conversationId: "other",
        kind: "presentation",
        presentations: ["call"],
      }),
      entry("standalone", {
        kind: "presentation",
        presentations: ["other-call"],
      }),
    ]);
    expect(projected).toHaveLength(3);
    expect(projected[0]?.presentations).toEqual([]);
  });

  it("keeps embedded cards and multiple calls independent", () => {
    const projected = combineToolActivities([
      entry("one", {
        content: "One",
        toolCalls: [call],
        presentations: ["call"],
      }),
      entry("two", {
        kind: "tool",
        toolCalls: [{ ...call, callId: "second" }],
      }),
      entry("view", { kind: "presentation", presentations: ["second"] }),
    ]);
    expect(projected).toHaveLength(2);
    expect(projected.map((item) => item.presentations)).toEqual([
      ["call"],
      ["second"],
    ]);
  });
});
