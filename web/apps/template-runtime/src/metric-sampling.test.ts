// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";
import { metricObservations } from "../../../packages/ui/src/observation-data";
import { metricOption } from "../../../packages/ui/src/metric-options";

afterEach(() => document.body.replaceChildren());
for (const locale of ["zh-CN", "en-US"])
  for (const [name, fixture] of Object.entries(fixtures.scenarios).filter(
    ([name]) => /^(metric|dashboard)-sampling-/.test(name),
  ))
    it(`preserves real chart inputs for ${name} in ${locale}`, async () => {
      const charts = presentTool(fixture.Tool, fixture.Data, locale);
      await vi.waitFor(() => expect(charts.length).toBeGreaterThan(0));
      const chart = charts.at(-1)!;
      const series = metricObservations(
        (chart.targets as { data: unknown }[])[0]!.data,
      );
      expect(series).toHaveLength(name.endsWith("multi") ? 2 : 1);
      for (const row of series) {
        expect(row.row.argus_statistics).toMatchObject({
          sample_count: 600,
          max: 10000,
        });
        if (name.endsWith("gap-budget")) {
          expect(row.points).toHaveLength(0);
          expect(row.row.argus_sample_preview).toMatchObject({
            state: "omitted_gap_budget",
          });
        } else {
          expect(row.points).toHaveLength(200);
          expect(row.points.filter((p) => p[1] === null)).toHaveLength(1);
          expect(Math.max(...row.points.map((p) => p[1] ?? -Infinity))).toBe(
            10000,
          );
          expect(row.points[0]![0]).toBe(Date.parse("2026-10-13T01:20:00Z"));
          expect(row.points.at(-1)![0]).toBe(
            Date.parse("2026-10-13T01:29:59Z"),
          );
        }
      }
      const option = metricOption("timeseries", series, true)!;
      for (const line of option.series as { connectNulls: boolean }[])
        expect(line.connectNulls).toBe(false);
    });
