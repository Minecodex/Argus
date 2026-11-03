// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { presentTool } from "./test-support";

afterEach(() => document.body.replaceChildren());
it.each(["zh-CN", "en-US"])(
  "shows partial dashboard suitability and metadata-only scope in %s",
  async (locale) => {
    presentTool("telemetry.dashboard.list", undefined, locale);
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("Database overview"),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "部分适用" : "Partially applicable",
    );
    expect(document.body.textContent).toContain("host_metrics");
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "未执行面板采样" : "panels were not sampled",
    );
    const expand = [...document.querySelectorAll("button")].find(
      (button) =>
        button.textContent === (locale === "zh-CN" ? "展开" : "Expand"),
    );
    expect(expand).toBeDefined();
    expand!.click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        locale === "zh-CN" ? "未执行采样" : "Not sampled",
      ),
    );
  },
);

it.each(["zh-CN", "en-US"])(
  "keeps complete no-match distinct from no telemetry in %s",
  async (locale) => {
    presentTool(
      "telemetry.dashboard.list",
      {
        items: [],
        complete: true,
        has_more: false,
        request_context: {
          resource_types: ["host"],
          signals: ["logs"],
          integrations: ["host_metrics"],
        },
      },
      locale,
    );
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        locale === "zh-CN" ? "本次结果完整" : "Complete for this request",
      ),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN"
        ? "空列表不代表目标没有遥测"
        : "An empty list does not prove telemetry is absent",
    );
  },
);
