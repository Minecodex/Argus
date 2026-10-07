// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ChartTypePicker, type ChartTypeOption } from "./chart-type-picker";
afterEach(cleanup);
it("keeps the chosen uncommon type discoverable, expands all types and prevents incompatible selection", () => {
  const options: ChartTypeOption[] = [
    "timeseries",
    "stat",
    "gauge",
    "bar",
    "pie",
    "histogram",
    "heatmap",
    "state_timeline",
    "scatter",
    "table",
    "bar_gauge",
  ].map((type) => ({
    type,
    label: type,
    state: type === "histogram" ? "incompatible" : "unverified",
    reason:
      type === "histogram"
        ? "Requires histogram buckets"
        : "Run before verification",
  }));
  const onChange = vi.fn();
  render(<ChartTypePicker value="pie" options={options} onChange={onChange} />);
  expect(screen.getByRole("button", { name: "pie" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(
    screen.queryByRole("button", { name: "histogram" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /(?:All 11|全部 11)/ }));
  expect(screen.getByRole("button", { name: "histogram" })).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "histogram" }),
  ).toHaveAccessibleDescription("Requires histogram buckets");
  fireEvent.click(screen.getByRole("button", { name: "histogram" }));
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "table" }));
  expect(onChange).toHaveBeenCalledWith("table");
});
