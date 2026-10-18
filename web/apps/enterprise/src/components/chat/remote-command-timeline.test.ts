import { expect, it } from "vitest";
import { coalesceRemoteCommands } from "./remote-command-timeline";
import type { ChatMessage } from "./chat-view-model";

const entry = (id: string, extra: Partial<ChatMessage>): ChatMessage => ({
  id,
  conversationId: "c",
  role: "assistant",
  content: "",
  createdAt: "2026-10-18T00:00:00Z",
  ...extra,
});
const command = (ref: string): ChatMessage["toolCalls"] => [
  {
    callId: ref,
    toolName: "connect",
    status: "success",
    startedAt: "2026-10-18T00:00:00Z",
    display: { mode: "action", actionRef: ref, facts: [], issues: [] },
  },
];

it("keeps evidence before the plan and implementation after intervening conversation", () => {
  const source = [
    entry("evidence", { kind: "tool", toolCalls: command("read") }),
    entry("evidence-approval", { kind: "action", pendingActionRefs: ["read"] }),
    entry("plan", { kind: "interaction", interactionId: "plan" }),
    entry("confirmation", { role: "user", content: "Execute" }),
    entry("thought", { content: "Now verify the target" }),
    entry("write-approval", { kind: "action", pendingActionRefs: ["write"] }),
    entry("write", { kind: "tool", toolCalls: command("write") }),
  ];
  const timeline = coalesceRemoteCommands(source);
  expect(timeline.map((x) => x.id)).toEqual([
    "evidence",
    "plan",
    "confirmation",
    "thought",
    "write-approval",
  ]);
  expect(timeline[0]?.pendingActionRefs).toEqual(["read"]);
  expect(timeline[4]?.toolCalls?.[0]?.callId).toBe("write");
  expect(source[0]?.pendingActionRefs).toBeUndefined();
  expect(coalesceRemoteCommands(timeline)).toEqual(timeline);
});

it("does not coalesce unrelated tools, questions or approvals", () => {
  const source = [
    entry("question", { kind: "interaction", interactionId: "q" }),
    entry("search", {
      kind: "tool",
      toolCalls: [{ ...command("a")![0]!, toolName: "tool.search" }],
    }),
    entry("approval", { kind: "action", pendingActionRefs: ["a"] }),
  ];
  expect(coalesceRemoteCommands(source).map((x) => x.id)).toEqual(
    source.map((x) => x.id),
  );
});
