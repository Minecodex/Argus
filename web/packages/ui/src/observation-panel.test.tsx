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
