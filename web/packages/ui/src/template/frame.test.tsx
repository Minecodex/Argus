// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ToolPresentationFrame } from "./frame";

vi.mock("./host", () => ({
  createTemplateHost: (
    _: HTMLIFrameElement,
    options: { onReady(): void; onResize(height: number): void },
  ) => {
    options.onReady();
    options.onResize(600);
    return { destroy: vi.fn(), setContext: vi.fn(), notifyData: vi.fn() };
  },
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
test("expansion exposes the full measured result with explicit expanded state", async () => {
  render(
    <ToolPresentationFrame
      origin="https://cards.example.test"
      presentation={{
        template_source: "<section/>",
        template_hash: "test",
        detail_data: {},
      }}
      locale="en-US"
      colorScheme="light"
      title="Alert result"
      loadingLabel="Loading"
      failureLabel="Failed"
      expandLabel="Expand details"
      collapseLabel="Collapse details"
    />,
  );
  const frame = screen.getByTitle("Alert result");
  await waitFor(() => expect(frame).toHaveStyle({ height: "480px" }));
  fireEvent.click(screen.getByRole("button", { name: "Expand details" }));
  await waitFor(() => expect(frame).toHaveStyle({ height: "600px" }));
  expect(
    screen.getByRole("button", { name: "Collapse details" }),
  ).toHaveAttribute("aria-expanded", "true");
});

test("document layout uses the measured height without a nested preview toggle", async () => {
  render(
    <ToolPresentationFrame
      origin="https://cards.example.test"
      presentation={{
        template_source: "<section/>",
        template_hash: "test",
        detail_data: {},
      }}
      locale="en-US"
      colorScheme="light"
      title="Host list"
      loadingLabel="Loading"
      failureLabel="Failed"
      expandLabel="Expand details"
      collapseLabel="Collapse details"
      layout="document"
    />,
  );
  await waitFor(() =>
    expect(screen.getByTitle("Host list")).toHaveStyle({ height: "600px" }),
  );
  expect(screen.queryByRole("button", { name: "Expand details" })).toBeNull();
});
