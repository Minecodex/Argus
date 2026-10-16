// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { DashboardDefinitionView } from "./dashboard-definition-view";
import { LocaleProvider } from "./locale";

afterEach(cleanup);
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
Element.prototype.getAnimations ??= () => [];
it("preserves layout and exposes queries without inventing samples or edit controls", () => {
  const definition = {
    layout: { row_height: 40 },
    panels: [
      {
        id: "cpu",
        title: "Host CPU",
        signal: "metrics",
        type: "timeseries",
        unit: "%",
        decimals: 2,
        layout: { x: 3, y: 2, w: 6, h: 5 },
        targets: [
          {
            id: "q",
            language: "promql",
            expression: "cpu_usage{host='db-01'}",
          },
        ],
      },
    ],
  };
  const before = JSON.stringify(definition);
  const { container } = render(
    <LocaleProvider locale="en-US" persist={false}>
      <DashboardDefinitionView
        definition={definition}
        metadata={{ version: 7 }}
      />
    </LocaleProvider>,
  );
  expect(container.querySelector(".argus-dashboard-tile")).toHaveStyle({
    gridColumn: "4 / span 6",
    gridRow: "3 / span 5",
  });
  expect(
    container.querySelector(
      "canvas,.argus-dashboard-move,.argus-dashboard-resize",
    ),
  ).toBeNull();
  expect(screen.getByText("Layout only — no query samples")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "View configuration" }));
  expect(screen.getByText("cpu_usage{host='db-01'}")).toBeVisible();
  fireEvent.click(screen.getByRole("tab", { name: "Display" }));
  expect(screen.getByText("Decimals")).toBeVisible();
  expect(JSON.stringify(definition)).toBe(before);
});
