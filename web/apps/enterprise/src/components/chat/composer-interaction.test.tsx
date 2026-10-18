// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { ConversationInteraction } from "@argus/api-client";
import "../../i18n";
import { ComposerInteraction } from "./composer-interaction";
afterEach(cleanup);
const base: ConversationInteraction = {
  id: "question",
  conversation_id: "chat",
  run_id: "run",
  family_id: "question",
  kind: "user_input",
  revision: 1,
  status: "pending",
  version: 1,
  content_hash: "a".repeat(64),
  content: {
    questions: [
      {
        id: "scope",
        header: "Scope",
        question: "Choose scope",
        options: [
          { label: "Owned target", description: "Use the fixture" },
          { label: "Custom", description: "Specify a target" },
        ],
      },
      {
        id: "details",
        header: "Details",
        question: "Any constraints?",
        options: [],
      },
    ],
  },
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};
test("questions require explicit submission and preserve choices across pages", async () => {
  const respond = vi.fn(async () => {});
  render(
    <ComposerInteraction
      interaction={base}
      onRespond={respond}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("radio", { name: /Owned target/ }));
  expect(respond).not.toHaveBeenCalled();
  fireEvent.click(screen.getAllByRole("button", { name: "下一题" }).at(-1)!);
  fireEvent.change(screen.getByRole("textbox", { name: "补充回答" }), {
    target: { value: "Only the temporary directory" },
  });
  fireEvent.click(screen.getByRole("button", { name: "上一题" }));
  expect(screen.getByRole("radio", { name: /Owned target/ })).toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: "提交回答" }));
  await waitFor(() => expect(respond).toHaveBeenCalledOnce());
  expect(respond.mock.calls[0]).toEqual([
    base.id,
    expect.objectContaining({
      action: "answer",
      content_hash: base.content_hash,
      version: 1,
      answers: {
        scope: { selected_option: "Owned target", skipped: false },
        details: { text: "Only the temporary directory", skipped: false },
      },
    }),
  ]);
});
test("collapse never fabricates an answer and skip records an unanswered question", async () => {
  const respond = vi.fn(async () => {}),
    close = vi.fn();
  render(
    <ComposerInteraction
      interaction={{
        ...base,
        content: { questions: base.content.questions!.slice(0, 1) },
      }}
      onRespond={respond}
      onClose={close}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "收起" }));
  expect(close).toHaveBeenCalledOnce();
  expect(respond).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "跳过" }));
  await waitFor(() => expect(respond).toHaveBeenCalledOnce());
  expect(respond.mock.calls[0]).toEqual([
    base.id,
    expect.objectContaining({ answers: { scope: { skipped: true } } }),
  ]);
});
test("plan feedback and implementation are distinct typed submissions", async () => {
  const respond = vi.fn(async () => {});
  render(
    <ComposerInteraction
      interaction={{
        ...base,
        kind: "plan",
        content: {
          title: "Fix fixture",
          description: "Scoped repair",
          steps: [{ description: "Verify" }],
        },
      }}
      onRespond={respond}
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "修改意见" }), {
    target: { value: "Add verification" },
  });
  fireEvent.click(screen.getByRole("button", { name: "提交修改意见" }));
  await waitFor(() => expect(respond).toHaveBeenCalledOnce());
  expect(respond.mock.calls[0]).toEqual([
    base.id,
    expect.objectContaining({ action: "revise", feedback: "Add verification" }),
  ]);
  fireEvent.change(screen.getByRole("textbox", { name: "修改意见" }), {
    target: { value: "" },
  });
  fireEvent.click(screen.getByRole("button", { name: "执行此计划" }));
  await waitFor(() => expect(respond).toHaveBeenCalledTimes(2));
  expect(respond.mock.calls[1]).toEqual([
    base.id,
    expect.objectContaining({ action: "confirm" }),
  ]);
});
test("a rejected response preserves the user's text for retry", async () => {
  const respond = vi.fn(async () => {
    throw new Error("Conflict");
  });
  render(
    <ComposerInteraction
      interaction={{
        ...base,
        content: { questions: [base.content.questions![1]!] },
      }}
      onRespond={respond}
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "补充回答" }), {
    target: { value: "Keep this answer" },
  });
  fireEvent.click(screen.getByRole("button", { name: "提交回答" }));
  await waitFor(() => expect(screen.getByRole("alert")).toBeVisible());
  expect(screen.getByRole("textbox", { name: "补充回答" })).toHaveValue(
    "Keep this answer",
  );
});
