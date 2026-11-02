// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
it.each(["zh-CN", "en-US"])(
  "shows source-qualified ratio semantics without changing raw unit in %s",
  async (locale) => {
    presentTool("observe.catalog.query", undefined, locale);
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("system_memory_utilization"),
    );
    const row = [...document.querySelectorAll("tr")].find(
      (entry) =>
        entry.textContent?.includes("system_memory_utilization") &&
        entry.querySelector("button"),
    );
    expect(row).toBeDefined();
    row!.querySelector("button")!.click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        locale === "zh-CN" ? "量值语义" : "Quantity semantics",
      ),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "百分比（原值 × 100）" : "Percent (raw × 100)",
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN"
        ? "原始类型和单位保持不变"
        : "Raw type and unit remain unchanged",
    );
    expect(row!.textContent).toContain("gauge");
    expect(row!.textContent).toContain("1");
  },
);
it.each(["zh-CN", "en-US"])(
  "shows partial capabilities without claiming healthy telemetry in %s",
  async (locale) => {
    presentTool("observe.capabilities.get", undefined, locale);
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("orders-mysql"),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "结果不完整" : "Partial",
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN"
        ? "本次能力查询未读取遥测样本；这不代表目标历史数据不存在。"
        : "This capability request did not read telemetry samples; it does not prove historical data is absent.",
    );
    const expand = [...document.querySelectorAll("button")].find(
      (button) =>
        button.textContent === (locale === "zh-CN" ? "展开" : "Expand"),
    );
    expect(expand).toBeDefined();
    expand!.click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        locale === "zh-CN"
          ? "未执行采样检查，请以实际查询为准"
          : "Sample check not performed; use actual query evidence",
      ),
    );
    for (const fact of [
      locale === "zh-CN" ? "配置采集器" : "Configured collector",
      locale === "zh-CN" ? "生效采集器" : "Effective collector",
      "a1111111-1111-4111-8111-111111111111",
      "b2222222-2222-4222-8222-222222222222",
      locale === "zh-CN"
        ? "此结果尚未核对主机或集群归属"
        : "Host or Cluster placement has not been verified by this result",
      locale === "zh-CN"
        ? "本节仅列采集实例；SDK 接入点和其他来源需另行核对。"
        : "This section lists collection instances only; SDK endpoints and other sources need separate checks.",
    ])
      expect(document.body.textContent).toContain(fact);
    for (const fact of [
      locale === "zh-CN" ? "本页来源数" : "Sources on this page",
      locale === "zh-CN" ? "本页版本项数" : "Version entries on this page",
      locale === "zh-CN"
        ? "本页：1 个来源，3 个版本项。"
        : "This page: 1 source, 3 version entries.",
      "c3333333-3333-4333-8333-333333333333:3",
    ])
      expect(document.body.textContent).toContain(fact);
  },
);
it("keeps source/type/unit and observed status visible in metric discovery", async () => {
  presentTool("observe.catalog.query", undefined, "en-US");
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("mysql_threads"),
  );
  for (const fact of ["gauge", "threads", "mysql", "Yes"])
    expect(document.body.textContent).toContain(fact);
});

it.each(["zh-CN", "en-US"])(
  "shows classic and native histogram query methods in %s",
  async (locale) => {
    presentTool("observe.catalog.query", undefined, locale);
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("request_duration"),
    );
    for (const name of ["request_duration", "request_size"]) {
      const row = [...document.querySelectorAll("tr")].find(
        (entry) =>
          entry.textContent?.includes(name) && entry.querySelector("button"),
      );
      expect(row).toBeDefined();
      row!.querySelector("button")!.click();
    }
    await vi.waitFor(() =>
      expect(
        document.getElementById("observation-discovery")!.textContent,
      ).toContain("histogram_count"),
    );
    const text = document.getElementById("observation-discovery")!.textContent;
    for (const value of [
      locale === "zh-CN" ? "经典直方图" : "Classic histogram",
      locale === "zh-CN" ? "原生直方图" : "Native histogram",
      '{__name__="request_duration_count"}',
      'histogram_count({__name__="request_size"})',
      "{observation}",
      locale === "zh-CN" ? "均值单位" : "Mean unit",
    ])
      expect(text).toContain(value);
  },
);

it.each(["zh-CN", "en-US"])(
  "explains incomplete supported-metric checks without claiming no data in %s",
  async (locale) => {
    presentTool(
      "observe.catalog.query",
      {
        metrics: [],
        complete: false,
        has_more: false,
        warnings: ["SUPPORTED_METRIC_OBSERVATION_CHECK_INCOMPLETE"],
      },
      locale,
    );
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        locale === "zh-CN"
          ? "部分指标的观测情况尚未查全"
          : "Observation checks for some metrics are incomplete",
      ),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN"
        ? "不能据此认定它们没有数据"
        : "does not establish that they have no data",
    );
    const visibleText = document.getElementById(
      "observation-discovery",
    )!.textContent;
    expect(visibleText).not.toContain(
      "SUPPORTED_METRIC_OBSERVATION_CHECK_INCOMPLETE",
    );
    expect(visibleText).not.toContain(
      locale === "zh-CN" ? "本次结果完整" : "Complete for this request",
    );
    expect(visibleText).not.toContain(
      locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
    );
  },
);

it.each(["zh-CN", "en-US"])(
  "shows continuation without the opaque cursor in business scope in %s",
  async (locale) => {
    presentTool(
      "observe.resources.list",
      {
        items: [],
        complete: true,
        request_context: {
          search: "same-name",
          cursor: "opaque-signed-pagination-token",
        },
      },
      locale,
    );
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("same-name"),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "续查页" : "Continuation page",
    );
    expect(document.body.textContent).not.toContain(
      "opaque-signed-pagination-token",
    );
  },
);
