import { expect, it } from "vitest";
import { chatMessageFromPublic } from "./chat-view-model";

it.each([null, false, 0, {}, []])(
  "preserves recorded tool input/output including %j",
  (value) => {
    const message = chatMessageFromPublic({
      message_id: "message",
      conversation_id: "conversation",
      role: "assistant",
      content: "",
      created_at: "2026-10-16T00:00:00Z",
      tool_calls: [
        {
          call_id: "call",
          tool_name: "host.list",
          status: "success",
          started_at: "2026-10-16T00:00:00Z",
          input: value,
          output: value,
        },
      ],
    });
    expect(message?.toolCalls?.[0]?.input).toEqual(value);
    expect(message?.toolCalls?.[0]?.output).toEqual(value);
  },
);
