import { describe, expect, it } from "vitest";
import type { ConversationEvent } from "@argus/api-client/contracts";
import actualEvents from "../../../e2e/fixtures/plan11-scope-user-events-real.json";
import { chatMessageFromPublic, chatMessagesFromEvents } from "./chat-view-model";
import { parseChatTaskScope } from "./task-scope";

describe("server-admitted task scope in historical messages", () => {
  it("preserves actual R67 restricted and reset events without rewriting history", () => {
    const before = JSON.stringify(actualEvents);
    const messages = chatMessagesFromEvents(actualEvents as unknown as ConversationEvent[]);
    expect(messages.map((message) => message.taskScope)).toEqual([
      "general_diagnosis", "dashboard_analysis", "general_diagnosis",
    ]);
    expect(messages[1]?.conversationId).toBe(messages[2]?.conversationId);
    expect(messages[1]?.content).toContain("/分析仪表盘");
    expect(messages[2]?.content).toContain("/综合诊断");
    expect(JSON.stringify(actualEvents)).toBe(before);
  });

  it("uses the same known enum for direct public messages", () => {
    for (const scope of ["general_diagnosis", "dashboard_analysis", "dashboard_authoring"]) {
      const message = chatMessageFromPublic({
        message_id: "message", conversation_id: "conversation", role: "user",
        content: "Do the requested work", created_at: "2026-11-01T17:00:00Z",
        dashboard_context: { mode: "analyze", dashboard_ids: [], task_scope: scope },
      });
      expect(message?.taskScope).toBe(scope);
    }
  });

  it("does not turn arbitrary values or user text into an admitted scope", () => {
    for (const value of [undefined, null, "unknown", 1, { task_scope: "dashboard_analysis" }])
      expect(parseChatTaskScope(value)).toBeUndefined();
    const event = structuredClone(actualEvents[1]!);
    delete (event.payload.dashboard_context as { task_scope?: string }).task_scope;
    const message = chatMessagesFromEvents([event] as unknown as ConversationEvent[])[0];
    expect(message?.content).toContain("/分析仪表盘");
    expect(message?.taskScope).toBeUndefined();
  });
});
