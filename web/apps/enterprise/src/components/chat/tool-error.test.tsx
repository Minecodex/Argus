// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "@argus/ui";
import i18n from "../../i18n";
import { ToolTrace } from "./tool-trace";
import { toolFailureText, requestedToolLabel } from "./tool-error";
import type { ToolCallTrace } from "./chat-view-model";

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("zh-CN");
});
const failed: ToolCallTrace = {
  callId: "call",
  toolName: "tool.invoke",
  source: "argus",
  requestedTarget: "dashboard/context.resolve",
  status: "failed",
  startedAt: "2026-11-16T01:00:00Z",
  output: {
    error_code: "DASHBOARD_CONDITION_EVIDENCE_REQUIRED",
    details: {
      issues: [
        {
          path: "/evidence/0/quote",
          constraint: "exact_excerpt_from_current_user_message",
        },
      ],
    },
  },
};
it.each(["zh-CN", "en-US"])(
  "renders a specific failed target and public field feedback in %s",
  async (locale) => {
    await i18n.changeLanguage(locale);
    render(
      <LocaleProvider locale={locale as "zh-CN" | "en-US"} persist={false}>
        <ToolTrace toolCalls={[failed]} />
      </LocaleProvider>,
    );
    const toggle = screen.getByRole("button");
    expect(toggle).toHaveTextContent("dashboard/context.resolve");
    expect(toggle).toHaveTextContent(
      locale === "zh-CN"
        ? "条件证据不符合要求"
        : "Condition evidence is invalid",
    );
    fireEvent.click(toggle);
    expect(screen.getByRole("table")).toHaveTextContent("/evidence/0/quote");
    expect(screen.getByRole("table")).toHaveTextContent(
      locale === "zh-CN" ? "必须逐字引用" : "exact contiguous excerpt",
    );
  },
);
it("does not promote external tool payloads into native errors or routing", () => {
  expect(
    toolFailureText(i18n.t, { ...failed, source: "external_mcp" }),
  ).toBeUndefined();
  expect(
    requestedToolLabel("external-tool", {
      category: "dashboard",
      name: "context.resolve",
    }),
  ).toBeUndefined();
  expect(
    requestedToolLabel("tool.invoke", {
      category: "dashboard",
      name: "<script>",
    }),
  ).toBeUndefined();
});
