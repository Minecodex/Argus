// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ChatMessageList } from "./message-list";
import type { ChatMessage } from "./chat-view-model";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: () => "Conversation messages" }),
}));
vi.mock("./message-item", () => ({
  ChatMessageItem: () => null,
  ChatAssistantHeading: () => null,
}));
afterEach(cleanup);

it("allows keyboard entry to the named message scroll region without action cards", () => {
  render(<ChatMessageList messages={[]} pendingUser={null} streaming={null} />);
  const history = screen.getByRole("region", { name: "Conversation messages" });
  history.focus();
  expect(history).toHaveFocus();
});

it("keeps a user's expansion choice through completion and prepended activity pages", () => {
  const message = (id: string, running = false): ChatMessage => ({
    id,
    conversationId: "conversation",
    runId: "run",
    role: "assistant",
    kind: "tool",
    content: "",
    createdAt: "2026-11-17T00:00:00Z",
    toolCalls: [
      {
        callId: id,
        toolName: "read",
        source: "sandbox",
        startedAt: "2026-11-17T00:00:00Z",
        status: running ? "running" : "success",
        rawStatus: running ? "running" : "succeeded",
      },
    ],
  });
  const { rerender } = render(
    <ChatMessageList
      messages={[message("a", true), message("b", true)]}
      pendingUser={null}
      streaming={null}
    />,
  );
  const toggle = () =>
    screen.getByTestId("tool-activity-group").querySelector("button")!;
  expect(toggle()).toHaveAttribute("aria-expanded", "true");
  fireEvent.click(toggle());
  rerender(
    <ChatMessageList
      messages={[message("earlier"), message("a"), message("b")]}
      pendingUser={null}
      streaming={null}
    />,
  );
  expect(toggle()).toHaveAttribute("aria-expanded", "false");
  fireEvent.click(toggle());
  rerender(
    <ChatMessageList
      messages={[message("earlier"), message("a"), message("b"), message("c")]}
      pendingUser={null}
      streaming={null}
    />,
  );
  expect(toggle()).toHaveAttribute("aria-expanded", "true");
});
