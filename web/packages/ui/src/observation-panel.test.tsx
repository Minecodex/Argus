// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ObservationPanel } from "./observation-panel";
import { LocaleProvider } from "./locale";

vi.mock("./observation-chart", () => ({
  ObservationChart: ({
    onSelect,
  }: {
    onSelect?: (index: number, series: number) => void;
  }) => <button onClick={() => onSelect?.(0, 0)}>Select chart datum</button>,
}));
afterEach(cleanup);

it.each(["zh-CN", "en-US"])(
  "explains omitted metric curves while retaining exact statistics in %s",
  (locale) => {
    localStorage.setItem("argus.locale", locale);
    const targets = [
      {
        id: "main",
        status: "success",
        result_type: "matrix",
        data: {
          result: [
            {
              metric: { __name__: "cpu" },
              values: [],
              argus_sample_preview: {
                state: "omitted_gap_budget",
                total: 600,
                shown: 0,
              },
              argus_statistics: {
                max: 10000,
                last: null,
                count: 300,
                sample_count: 600,
              },
            },
          ],
        },
      },
    ];
    const tree = (type: string, reducer: "max" | "last" = "max") => (
      <LocaleProvider>
        <ObservationPanel
          title="Metric"
          type={type}
          signal="metrics"
          display={{ reducer }}
          targets={targets}
        />
      </LocaleProvider>
    );
    const { rerender } = render(tree("timeseries"));
    expect(
      screen.getByText(
        locale === "zh-CN"
          ? /缺测边界超过预览点数预算/
          : /Missing boundaries exceed/,
      ),
    ).toBeVisible();
    expect(
      screen.getByText(
        locale === "zh-CN" ? /曲线预览不可用/ : /Chart preview unavailable/,
      ),
    ).toBeVisible();
    expect(
      screen.queryByText(
        locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
      ),
    ).not.toBeInTheDocument();
    rerender(tree("stat"));
    expect(screen.getByText("10000.00")).toBeVisible();
    rerender(tree("stat", "last"));
    expect(screen.getByText("—")).toBeVisible();
    expect(
      screen.queryByText(
        locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
      ),
    ).not.toBeInTheDocument();
  },
);

it.each(["zh-CN", "en-US"])(
  "shows trace entry names, duration units and full cell tooltips in %s",
  (locale) => {
    localStorage.setItem("argus.locale", locale);
    const onSelect = vi.fn();
    const row = {
      rootService: "orders-api",
      rootOperation: "POST /orders/with/a/long/entry/path",
      duration: 12.345,
      status: "error",
      startTime: "2026-10-14T00:00:00Z",
      spanCount: 3,
      errorCount: 1,
      traceId: "trace-full-identity",
      sourceId: "source-full-identity",
    };
    render(
      <LocaleProvider>
        <ObservationPanel
          title="Traces"
          type="trace_list"
          signal="traces"
          targets={[
            {
              id: "main",
              status: "success",
              result_type: "trace_list",
              data: { queryTraces: { traces: [row] } },
            },
          ]}
          onSelect={onSelect}
        />
      </LocaleProvider>,
    );
    const headings = screen.getAllByRole("columnheader");
    expect(headings[0]).toHaveTextContent(
      locale === "zh-CN" ? "入口服务" : "Entry service",
    );
    expect(screen.getByText("12.35 ms")).toBeVisible();
    expect(
      screen.getByRole("region", {
        name: locale === "zh-CN" ? "查询结果表格" : "Query result table",
      }),
    ).toHaveAttribute("tabindex", "0");
    expect(screen.getByText(row.rootOperation)).toHaveAttribute(
      "title",
      row.rootOperation,
    );
    expect(
      screen.queryByRole("columnheader", { name: "sourceId" }),
    ).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", {
        name: locale === "zh-CN" ? "打开详情" : "Open details",
      }),
    );
    expect(onSelect).toHaveBeenCalledWith(row, "main");
  },
);

it.each(["zh-CN", "en-US"])(
  "presents APM sample statistics in %s without losing drilldown identities",
  (locale) => {
    localStorage.setItem("argus.locale", locale);
    const onSelect = vi.fn();
    const row = {
      serviceName: "argus-server",
      sampleCount: 4,
      errorCount: 1,
      errorRate: 0.25,
      durationMeanMs: 12.345,
      durationP95Ms: null,
      sourceId: "registered-source",
      resourceId: "authorized-resource",
      timestamp: "2026-09-25T00:00:00Z",
    };
    render(
      <LocaleProvider>
        <ObservationPanel
          title="Services"
          type="apm_services"
          signal="traces"
          targets={[
            {
              id: "main",
              status: "success",
              result_type: "apm_services",
              data: {
                queryAPMServices: {
                  basis: "received_entry_spans",
                  rows: [row],
                },
              },
            },
          ]}
          onSelect={onSelect}
        />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("columnheader", {
        name: locale === "zh-CN" ? "服务" : "Service",
      }),
    ).toBeVisible();
    expect(
      screen.getByRole("columnheader", {
        name: locale === "zh-CN" ? "样本错误率" : "Sample error rate",
      }),
    ).toBeVisible();
    expect(
      screen.queryByRole("columnheader", { name: "sourceId" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("25.00%")).toBeVisible();
    expect(screen.getByText("12.35 ms")).toBeVisible();
    expect(screen.getByText("—")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", {
        name: locale === "zh-CN" ? "打开详情" : "Open details",
      }),
    );
    expect(onSelect).toHaveBeenCalledWith(row, "main");
  },
);

it("explains omitted negative pie slices and drills into the series selected by the same reduction", () => {
  localStorage.setItem("argus.locale", "en-US");
  const onSelect = vi.fn();
  const negative = {
    metric: { host: "a" },
    values: [
      [1, "-10"],
      [2, "2"],
    ],
  };
  const positive = {
    metric: { host: "b" },
    values: [
      [1, "10"],
      [2, "-2"],
    ],
  };
  render(
    <LocaleProvider>
      <ObservationPanel
        title="Pie"
        type="pie"
        signal="metrics"
        display={{ reducer: "mean" }}
        targets={[
          {
            id: "main",
            status: "success",
            result_type: "matrix",
            data: [negative, positive],
          },
        ]}
        onSelect={onSelect}
      />
    </LocaleProvider>,
  );
  expect(screen.getByRole("status")).toHaveTextContent("omitted series: 1");
  fireEvent.click(screen.getByRole("button", { name: "Select chart datum" }));
  expect(onSelect).toHaveBeenCalledWith(positive, "main");
});

it("keeps a missing last Stat uncolored and shows a sample-mean explanation only when explicitly selected", () => {
  localStorage.setItem("argus.locale", "en-US");
  const targets = [
    {
      id: "main",
      status: "success",
      result_type: "matrix",
      data: [
        {
          values: [
            [1, "2"],
            [2, "4"],
            [3, "NaN"],
          ],
        },
      ],
    },
  ];
  const { container, rerender } = render(
    <LocaleProvider>
      <ObservationPanel
        title="Stat"
        type="stat"
        signal="metrics"
        targets={targets}
        thresholds={[{ value: 1, tone: "danger" }]}
      />
    </LocaleProvider>,
  );
  expect(container.querySelector("strong")).toHaveTextContent("—");
  expect(container.querySelector("strong")).not.toHaveAttribute("data-tone");
  const region = screen.getByRole("region", { name: "Stat · Statistics" });
  region.focus();
  expect(region).toHaveFocus();
  rerender(
    <LocaleProvider>
      <ObservationPanel
        title="Stat"
        type="stat"
        signal="metrics"
        targets={targets}
        display={{ reducer: "mean" }}
        thresholds={[{ value: 1, tone: "danger" }]}
      />
    </LocaleProvider>,
  );
  expect(container.querySelector("strong")).toHaveTextContent("3.00");
  expect(container.querySelector("strong")).toHaveAttribute(
    "data-tone",
    "danger",
  );
  expect(screen.getByText(/Sample mean, not time weighted/)).toBeVisible();
});
