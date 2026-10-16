// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationListItem } from "./conversation-list-item";
afterEach(cleanup);
it("retains the selectable title and timestamp without an icon", () => {
  const onPress = vi.fn();
  const { container } = render(
    <ConversationListItem
      title="主机诊断"
      meta="10/12"
      selected
      onPress={onPress}
    />,
  );
  const row = screen.getByRole("button", { name: "主机诊断 10/12" });
  expect(row).toHaveAttribute("aria-current", "page");
  expect(container.querySelector("svg")).toBeNull();
  fireEvent.click(row);
  expect(onPress).toHaveBeenCalledOnce();
});
