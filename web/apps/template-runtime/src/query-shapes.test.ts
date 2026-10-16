// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
const cases = fixtures.scenarios as Record<
  string,
  { Tool: string; Data: Record<string, unknown> }
>;
for (const locale of ["zh-CN", "en-US"]) {
  const zh = locale === "zh-CN";
  for (const health of ["no_data", "error"])
    for (const kind of ["events", "history", "detail"]) {
      it(`health episode has no metric threshold ${health}/${kind}/${locale}`, async () => {
        const scenario = cases[`event-kind-${health}-${kind}`]!;
        presentTool(scenario.Tool, scenario.Data, locale);
        if (kind !== "detail") {
          await vi.waitFor(() =>
            expect(document.querySelector("tbody")?.textContent).toContain(
              zh ? "不适用（健康事件）" : "Not applicable (health event)",
            ),
          );
          expect(document.querySelector("tbody")?.textContent).not.toContain(
            "— / 0",
          );
          (document.querySelector("tbody button") as HTMLButtonElement).click();
        }
        await vi.waitFor(() =>
          expect(
            document.querySelector("[data-alert-health]")?.textContent,
          ).toContain(zh ? "无指标观测值" : "no metric observation"),
        );
        expect(
          [...document.querySelectorAll("dt")].map((n) => n.textContent),
        ).not.toContain(zh ? "触发值 / 阈值" : "Trigger value / threshold");
      });
    }
  for (const kind of ["table", "timeseries", "zero", "null"]) {
    it(`KQL exposes actual aggregate columns ${kind}/${locale}`, async () => {
      const scenario = cases[`log-aggregate-${kind}`]!;
      presentTool(scenario.Tool, scenario.Data, locale);
      await vi.waitFor(() =>
        expect(document.querySelector("tbody")?.textContent).toContain(
          "orders-api",
        ),
      );
      const headers = [...document.querySelectorAll("thead th")].map(
        (n) => n.textContent,
      );
      const index = headers.indexOf(zh ? "计数" : "Count");
      expect(index).toBeGreaterThanOrEqual(0);
      expect(
        document.querySelector("tbody tr")!.children[index]!.textContent,
      ).toBe(kind === "zero" ? "0" : kind === "null" ? "—" : "427");
    });
  }
  for (const [name, scenario] of Object.entries(cases).filter(
    ([name]) => name.startsWith("trace-root-") || name === "trace-alias-multi",
  )) {
    it(`renders the selected GraphQL result ${name}/${locale}`, async () => {
      const charts = presentTool(scenario.Tool, scenario.Data, locale);
      await vi.waitFor(() =>
        expect(document.body.textContent).toContain("orders-api"),
      );
      expect(document.body.textContent).not.toContain(
        zh ? "当前范围没有数据" : "No data in this range",
      );
      if (name.endsWith("queryAPMTopology"))
        expect(charts.some((c) => c.type === "apm_topology")).toBe(true);
      if (name.endsWith("queryTraceGraph") || name.endsWith("queryTrace"))
        expect(charts.some((c) => c.type === "trace_detail")).toBe(true);
      if (name === "trace-alias-multi")
        expect(
          document.querySelectorAll("[data-trace-result-kind]"),
        ).toHaveLength(2);
    });
  }
}
