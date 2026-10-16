// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ObservationChart } from "./observation-chart";
const mocks = vi.hoisted(() => ({
  init: vi.fn(),
  paint: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
}));
vi.mock("echarts/core", () => ({ use: vi.fn(), init: mocks.init }));
vi.mock("./theme", () => ({ useTheme: () => ({ resolvedTheme: "light" }) }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it("waits for a visible chart rectangle and resumes after a hidden iframe is shown", () => {
  let width = 0,
    height = 0,
    notify = () => {};
  vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(
    () => width,
  );
  vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
    () => height,
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        notify = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  mocks.init.mockReturnValue({
    setOption: mocks.paint,
    resize: mocks.resize,
    dispose: mocks.dispose,
    on: vi.fn(),
  });
  const view = render(
    <ObservationChart
      label="topology"
      option={{ series: [{ type: "graph" }] }}
    />,
  );
  expect(mocks.init).not.toHaveBeenCalled();
  width = 600;
  height = 280;
  act(() => notify());
  expect(mocks.init).toHaveBeenCalledTimes(1);
  expect(mocks.paint).toHaveBeenCalled();
  width = 0;
  act(() => notify());
  expect(mocks.resize).not.toHaveBeenCalled();
  width = 600;
  act(() => notify());
  expect(mocks.resize).toHaveBeenCalledTimes(1);
  view.unmount();
  expect(mocks.dispose).toHaveBeenCalledTimes(1);
});
