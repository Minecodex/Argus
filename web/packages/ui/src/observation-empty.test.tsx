// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ObservationPanel } from "./observation-panel";
import { LocaleProvider } from "./locale";

vi.mock("./observation-chart", () => ({
  ObservationChart: () => <div role="img" aria-label="Rendered chart" />,
}));
afterEach(cleanup);

const shapes = [
  { type: "trace_detail", data: { queryTrace: { spans: [], edges: [] } } },
  {
    type: "apm_topology",
    data: { queryAPMTopology: { nodes: [], edges: [] } },
  },
  { type: "apm_red", data: { queryAPMRED: { rows: [] } } },
  { type: "apm_services", data: { queryAPMServices: { rows: [] } } },
];
for (const locale of ["zh-CN", "en-US"] as const)
  for (const shape of shapes) {
    it(`empty ${shape.type} is explicit without a chart or zero value in ${locale}`, () => {
      render(
        <LocaleProvider locale={locale} persist={false}>
          <ObservationPanel
            title="Observed results"
            type={shape.type}
            signal="traces"
            targets={[
              {
                id: "A",
                status: "success",
                result_type: shape.type,
                data: shape.data,
              },
            ]}
          />
        </LocaleProvider>,
      );
      expect(
        screen.getByText(
          locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
        ),
      ).toBeVisible();
      expect(screen.queryByRole("img")).not.toBeInTheDocument();
      expect(screen.queryByText("0.00")).not.toBeInTheDocument();
    });
  }
