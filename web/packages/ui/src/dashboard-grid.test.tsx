// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DashboardGrid } from "./dashboard-grid";
import { LocaleProvider } from "./locale";

beforeEach(() => {
  localStorage.setItem("argus.locale", "en-US");
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1200);
  vi.stubGlobal(
    "PointerEvent",
    class extends MouseEvent {
      pointerId: number;
      constructor(type: string, init: PointerEventInit = {}) {
        super(type, init);
        this.pointerId = init.pointerId ?? 1;
      }
    },
  );
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", {
    configurable: true,
    value: vi.fn(),
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup() {
  const change = vi.fn();
  const items = [
    {
      id: "a",
      title: "A",
      layout: { x: 0, y: 0, w: 6, h: 24, min_w: 3, min_h: 12 },
    },
  ];
  render(
    <LocaleProvider>
      <button>Other control</button>
      <DashboardGrid items={items} rowHeight={8} editable onChange={change}>
        {(item) => <span>{item.title}</span>}
      </DashboardGrid>
    </LocaleProvider>,
  );
  return {
    change,
    handle: screen.getByRole("button", { name: "Move panel A" }),
    tile: document.querySelector('[data-panel-id="a"]')!,
  };
}

it("focuses a pointer gesture so Escape discards the entire preview", () => {
  const { change, handle, tile } = setup();
  screen.getByRole("button", { name: "Other control" }).focus();
  fireEvent.pointerDown(handle, {
    button: 0,
    pointerId: 1,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 300, clientY: 116 });
  expect(handle).toHaveFocus();
  expect(tile).toHaveStyle({
    gridColumn: "3 / span 6",
    gridRow: "3 / span 24",
  });
  fireEvent.keyDown(handle, { key: "Escape" });
  fireEvent.pointerUp(handle, { pointerId: 1 });
  expect(tile).toHaveStyle({
    gridColumn: "1 / span 6",
    gridRow: "1 / span 24",
  });
  expect(change).not.toHaveBeenCalled();
});

it("ignores another pointer and cancels on capture loss without committing or reusing old preview", () => {
  const { change, handle, tile } = setup();
  fireEvent.pointerDown(handle, {
    button: 0,
    pointerId: 1,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerMove(handle, { pointerId: 2, clientX: 400, clientY: 200 });
  fireEvent.pointerUp(handle, { pointerId: 2 });
  expect(change).not.toHaveBeenCalled();
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 200, clientY: 100 });
  fireEvent.lostPointerCapture(handle, { pointerId: 1 });
  expect(tile).toHaveStyle({ gridColumn: "1 / span 6" });
  fireEvent.pointerDown(handle, {
    button: 0,
    pointerId: 1,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 99, clientY: 100 });
  fireEvent.pointerUp(handle, { pointerId: 1 });
  expect(change).not.toHaveBeenCalled();
});
