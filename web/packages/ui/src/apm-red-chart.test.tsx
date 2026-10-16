// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { APMRedChart } from "./apm-red-chart";
const state = vi.hoisted(() => ({
  option: undefined as Record<string, unknown> | undefined,
}));
vi.mock("./observation-chart", () => ({
  ObservationChart: ({ option }: { option: Record<string, unknown> }) => {
    state.option = option;
    return null;
  },
}));
vi.mock("./select", () => ({ Select: () => null }));
afterEach(cleanup);
it("defaults to an observed metric when a partial GraphQL projection omits sample rate", () => {
  render(
    <APMRedChart
      title="P95"
      legend
      rows={[
        {
          target: "sample",
          row: {
            serviceName: "orders",
            timestamp: "2026-10-14T00:00:00Z",
            durationP95Ms: 42,
          },
        },
      ]}
    />,
  );
  expect((state.option!.series as { data: unknown[] }[])[0]!.data).toEqual([
    ["2026-10-14T00:00:00Z", 42],
  ]);
});
it.each([{ values: [7] }, { values: [null, 7] }, { values: [7, 8] }])(
  "keeps a lone valid RED sample visible: %j",
  ({ values }) => {
    render(
      <APMRedChart
        title="RED"
        legend
        rows={values.map((value, index) => ({
          target: "sample",
          row: {
            serviceName: "orders",
            timestamp: new Date(1800000000000 + index * 60000).toISOString(),
            samplesPerSecond: value,
          },
        }))}
      />,
    );
    const series = (
      state.option!.series as {
        showSymbol: boolean;
        connectNulls: boolean;
        data: unknown[];
      }[]
    )[0]!;
    expect(series.showSymbol).toBe(
      values.filter((v) => v !== null).length === 1,
    );
    expect(series.connectNulls).toBe(false);
    expect(series.data).toHaveLength(values.length);
  },
);
