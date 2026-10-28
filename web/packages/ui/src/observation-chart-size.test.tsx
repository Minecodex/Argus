// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ObservationChart } from "./observation-chart";
import { ObservationChartLink } from "./observation-chart-link";
const mocks = vi.hoisted(() => ({
  init: vi.fn(),
  paint: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
  action: vi.fn(),
}));
vi.mock("echarts/core", () => ({
  use: vi.fn(),
  init: mocks.init,
  connect: vi.fn(),
}));
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
  const chart = {
    group: "",
    dispatchAction: mocks.action,
    setOption: mocks.paint,
    resize: mocks.resize,
    dispose: mocks.dispose,
    on: vi.fn(),
  };
  mocks.init.mockReturnValue(chart);
  const view = render(
    <ObservationChartLink group="linked">
      <ObservationChart
        label="topology"
        option={{ series: [{ type: "graph" }] }}
      />
    </ObservationChartLink>,
  );
  expect(mocks.init).not.toHaveBeenCalled();
  width = 600;
  height = 280;
  act(() => notify());
  expect(mocks.init).toHaveBeenCalledTimes(1);
  expect(mocks.paint).toHaveBeenCalled();
  expect(chart.group).toBe("linked");
  width = 0;
  act(() => notify());
  expect(mocks.resize).not.toHaveBeenCalled();
  expect(mocks.action).toHaveBeenCalledWith({ type: "hideTip" });
  expect(chart.group).toBe("");
  width = 600;
  act(() => notify());
  expect(mocks.resize).toHaveBeenCalledTimes(1);
  expect(chart.group).toBe("linked");
  view.unmount();
  expect(mocks.dispose).toHaveBeenCalledTimes(1);
});
