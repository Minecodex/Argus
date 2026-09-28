// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ChatMessageList } from "./message-list";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: () => "Conversation messages" }),
}));
vi.mock("./message-item", () => ({ ChatMessageItem: () => null }));
afterEach(cleanup);

it("allows keyboard entry to the named message scroll region without action cards", () => {
  render(<ChatMessageList messages={[]} pendingUser={null} streaming={null} />);
  const history = screen.getByRole("region", { name: "Conversation messages" });
  history.focus();
  expect(history).toHaveFocus();
});
