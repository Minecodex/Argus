// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LogViewer } from "./log-viewer";
import { LocaleProvider } from "./locale";
afterEach(cleanup);

it("keeps a reader's position while new entries arrive and resumes following at the bottom", () => {
  const draw = (contents: string[]) => (
    <LocaleProvider locale="en-US">
      <LogViewer lines={contents.map((content) => ({ content }))} />
    </LocaleProvider>
  );
  const { rerender } = render(draw(["first"]));
  const output = screen.getByRole("log");
  let top = 0,
    height = 500;
  Object.defineProperties(output, {
    clientHeight: { get: () => 100 },
    scrollHeight: { get: () => height },
    scrollTop: {
      get: () => top,
      set: (value) => {
        top = Math.min(value, height - 100);
      },
    },
  });
  output.scrollTop = 400;
  fireEvent.scroll(output);
  expect(screen.getByRole("switch", { name: "Follow" })).toBeChecked();
  output.scrollTop = 60;
  fireEvent.scroll(output);
  expect(screen.getByRole("switch", { name: "Follow" })).not.toBeChecked();
  height = 550;
  rerender(draw(["first", "second"]));
  expect(output.scrollTop).toBe(60);
  expect(output).toHaveAttribute("aria-live", "off");
  output.scrollTop = 450;
  fireEvent.scroll(output);
  height = 600;
  rerender(draw(["first", "second", "third"]));
  expect(output.scrollTop).toBe(500);
  expect(output).toHaveAttribute("aria-live", "polite");
});

it("freezes displayed content when paused and exposes resume as the next action", () => {
  const draw = (contents: string[]) => (
    <LocaleProvider locale="en-US">
      <LogViewer lines={contents.map((content) => ({ content }))} />
    </LocaleProvider>
  );
  const { rerender } = render(draw(["first"]));
  fireEvent.click(screen.getByRole("button", { name: "Pause" }));
  rerender(draw(["first", "second"]));
  expect(screen.getByRole("log")).not.toHaveTextContent("second");
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  expect(screen.getByRole("log")).toHaveTextContent("second");
});

it("renders an honest empty state without a synthetic numbered log line", () => {
  const view = render(
    <LocaleProvider locale="en-US">
      <LogViewer
        lines={[]}
        emptyMessage="The container has no retained output"
      />
    </LocaleProvider>,
  );
  expect(screen.getByRole("status")).toHaveTextContent("no retained output");
  expect(view.container.querySelectorAll(".argus-log-viewer__ln")).toHaveLength(
    0,
  );
});
