// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChatMessageItem } from "./message-item";
import type { ChatMessage } from "./chat-view-model";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh-CN" },
  }),
}));
vi.mock("./conversation-file", () => ({
  MessageFiles: ({ items }: { items: { name: string }[] }) => (
    <div>{items.map((item) => item.name).join(", ")}</div>
  ),
}));
vi.mock("./tool-trace", () => ({
  ToolTrace: () => <div data-testid="tool-trace">Tool result</div>,
}));

afterEach(cleanup);
const message = (patch: Partial<ChatMessage>): ChatMessage => ({
  id: "message",
  conversationId: "conversation",
  role: "user",
  content: "",
  createdAt: "2026-10-12T01:00:00Z",
  ...patch,
});

describe("message content", () => {
  it("renders user text and time without a user avatar", () => {
    const { container } = render(
      <ChatMessageItem message={message({ content: "检查 CPU" })} />,
    );
    expect(screen.getByTestId("chat-message-user")).toHaveTextContent(
      "检查 CPU",
    );
    expect(container.querySelector("time")).not.toBeNull();
    expect(container.querySelector(".argus-avatar")).toBeNull();
  });

  it.each(["user", "assistant"] as const)(
    "does not render an empty %s reply",
    (role) => {
      const { container } = render(
        <ChatMessageItem message={message({ role, content: " \n " })} />,
      );
      expect(container).toBeEmptyDOMElement();
    },
  );

  it("renders a file-only message as an activity without a user heading or empty bubble", () => {
    const { container } = render(
      <ChatMessageItem
        message={message({
          files: [{ id: "file", name: "manifest.json", byte_size: 10 }],
        })}
      />,
    );
    expect(screen.getByTestId("chat-file-event")).toHaveTextContent(
      "manifest.json",
    );
    expect(screen.queryByTestId("chat-message-user")).not.toBeInTheDocument();
    expect(container.querySelector(".argus-chat-message__body")).toBeNull();
  });

  it("preserves a tool result without rendering an empty assistant body", () => {
    const { container } = render(
      <ChatMessageItem
        message={message({
          role: "assistant",
          toolCalls: [
            {
              callId: "call",
              toolName: "read",
              status: "success",
              startedAt: "2026-10-12T01:00:00Z",
            },
          ],
        })}
      />,
    );
    expect(screen.getByTestId("tool-trace")).toBeVisible();
    expect(container.querySelector(".argus-chat-message__body")).toBeNull();
  });
});
