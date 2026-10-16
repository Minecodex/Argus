import { expect, it } from "vitest";
import type { ChatMessage } from "./chat-view-model";
import { groupChatReplies } from "./reply-groups";
import { mergeTimeline } from "./live-timeline";

const message = (
  id: string,
  patch: Partial<ChatMessage> = {},
): ChatMessage => ({
  id,
  conversationId: "conversation",
  role: "assistant",
  content: id,
  createdAt: "2026-10-13T01:00:00Z",
  ...patch,
});

it("keeps prose, tools, files and later Runs in one reply until actual user input", () => {
  const timeline = [
    message("user-1", { role: "user" }),
    message("planning", { runId: "run-1" }),
    message("tool", {
      kind: "tool",
      content: "",
      toolCalls: [
        {
          callId: "call",
          toolName: "tool.search",
          status: "success",
          startedAt: "2026-10-13T01:00:01Z",
        },
      ],
    }),
    message("continuation", { runId: "run-2" }),
    message("file", {
      role: "system",
      kind: "file",
      content: "",
      files: [{ id: "file", name: "report.md", byte_size: 10 }],
    }),
    message("empty", { content: " \n" }),
    message("user-2", { role: "user" }),
    message("answer"),
  ];
  const groups = groupChatReplies(timeline);
  expect(groups.map((group) => group.role)).toEqual([
    "user",
    "assistant",
    "user",
    "assistant",
  ]);
  expect(groups[1]?.entries.map((entry) => entry.id)).toEqual([
    "planning",
    "tool",
    "continuation",
    "file",
  ]);
  expect(groups[3]?.entries.map((entry) => entry.id)).toEqual(["answer"]);
  expect(timeline[1]?.content).toBe("planning");
});

it("adds streamed continuation to the existing reply and deduplicates its persisted segment", () => {
  const first = message("planning", { runId: "run" });
  const live = message("live", {
    runId: "run",
    streamEntries: [
      first,
      message("next", { runId: "run", isStreaming: true }),
    ],
  });
  const groups = groupChatReplies(
    mergeTimeline([message("user", { role: "user" }), first], live),
  );
  expect(groups).toHaveLength(2);
  expect(groups[1]?.entries.map((entry) => entry.id)).toEqual([
    "planning",
    "next",
  ]);
});

it("treats a sent attachment as user input while automatic file activities do not split a reply", () => {
  const groups = groupChatReplies([
    message("first"),
    message("attachment", {
      role: "user",
      content: "",
      files: [{ id: "upload", name: "input.txt", byte_size: 1 }],
    }),
    message("second"),
  ]);
  expect(groups.map((group) => group.role)).toEqual([
    "assistant",
    "user",
    "assistant",
  ]);
});
