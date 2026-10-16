// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
const scenarios = fixtures.scenarios as Record<
  string,
  { Tool: string; Data: Record<string, unknown> }
>;
for (const locale of ["zh-CN", "en-US"]) {
  const zh = locale === "zh-CN";
  for (const [name, scenario] of Object.entries(scenarios).filter(([name]) =>
    name.startsWith("apm-limit-"),
  )) {
    it(`exposes business limit independently of display pagination ${name}/${locale}`, async () => {
      presentTool(scenario.Tool, scenario.Data, locale);
      const state = name.split("-").at(-1)!;
      await vi.waitFor(() =>
        expect(
          document
            .querySelector("[data-apm-limited]")
            ?.getAttribute("data-apm-limited"),
        ).toBe(state),
      );
      expect(document.body.textContent).toContain(
        state === "true"
          ? zh
            ? "已受业务限量截断"
            : "Limited by the business query"
          : state === "false"
            ? zh
              ? "未被业务限量截断"
              : "Not truncated by the business limit"
            : zh
              ? "是否受限未知"
              : "business limit unknown",
      );
      if (state === "true")
        expect(document.body.textContent).toContain(
          zh
            ? "更多业务结果需要让 AI 继续查询"
            : "ask AI to query additional business results",
        );
    });
  }
  for (const kind of ["single", "multi"]) {
    it(`draws the whole returned RED window and keeps exact table pages ${kind}/${locale}`, async () => {
      const scenario = scenarios["apm-red-large-" + kind]!;
      const charts = presentTool(scenario.Tool, scenario.Data, locale);
      await vi.waitFor(() =>
        expect(charts.some((spec) => spec.type === "apm_red")).toBe(true),
      );
      const chart = charts
        .filter((spec) => spec.type === "apm_red")
        .at(-1)! as {
        targets: {
          data: {
            rows: Record<string, unknown>[];
            chart_preview: {
              shown_rows: number;
              returned_rows: number;
              series: { points: Record<string, unknown>[] }[];
            };
          };
        }[];
      };
      const data = chart.targets[0]!.data;
      expect(data.rows.length).toBeLessThanOrEqual(200);
      expect(data.chart_preview.returned_rows).toBe(
        kind === "multi" ? 1200 : 600,
      );
      for (const group of data.chart_preview.series) {
        const window = scenario.Data.query_context as {
          from: string;
          to: string;
        };
        expect(Date.parse(String(group.points[0]!.timestamp))).toBe(
          Date.parse(window.from),
        );
        expect(Date.parse(String(group.points.at(-1)!.timestamp))).toBe(
          Date.parse(window.to) - 1000,
        );
        expect(group.points.some((row) => row.samplesPerSecond === 999)).toBe(
          true,
        );
        expect(group.points.some((row) => row.errorRate === 0.99)).toBe(true);
        expect(group.points.some((row) => row.durationP95Ms === 9999)).toBe(
          true,
        );
      }
      const result = (scenario.Data.result as { rows: unknown[] }[])[0]!;
      expect(result.rows.length).toBe(kind === "multi" ? 1200 : 600);
    });
  }
  it(`does not describe an execution partial as complete ${locale}`, async () => {
    const scenario = scenarios["apm-execution-partial"]!;
    presentTool(scenario.Tool, scenario.Data, locale);
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        zh ? "结果或预览不完整" : "Partial result or preview",
      ),
    );
    expect(document.body.textContent).toContain(
      zh ? "未被业务限量截断" : "Not truncated by the business limit",
    );
  });
}
