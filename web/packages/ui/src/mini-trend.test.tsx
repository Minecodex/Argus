// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { MiniTrend } from "./mini-trend";
import { LocaleProvider } from "./locale";

afterEach(cleanup);
const points = [
  { time: "2026-11-13T12:00:00Z", value: 10 },
  { time: "2026-11-13T12:00:30Z", value: null },
  { time: "2026-11-13T12:01:00Z", value: 30 },
];
function show() {
  return render(
    <LocaleProvider locale="en-US">
      <MiniTrend
        points={points}
        label="Memory trend"
        latest="30%"
        reference={100}
        renderDetail={(point) => (
          <span>
            {point.time} · {point.value ?? "missing"}
          </span>
        )}
      />
    </LocaleProvider>,
  );
}

it("opens actual tooltip content on mouse hover through the shared button", async () => {
  show();
  const trigger = screen.getByRole("button", { name: "Memory trend" });
  expect(trigger.tagName).toBe("BUTTON");
  fireEvent.mouseEnter(trigger);
  const tooltip = await screen.findByRole("tooltip");
  expect(tooltip).toHaveTextContent(points[2]!.time);
  fireEvent.mouseLeave(trigger);
  await waitFor(() =>
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument(),
  );
});

it("inspects points with the keyboard and keeps a missing sample as a gap", async () => {
  const view = show();
  const trigger = screen.getByRole("button", { name: "Memory trend" });
  fireEvent.mouseEnter(trigger);
  await screen.findByRole("tooltip");
  fireEvent.keyDown(trigger, { key: "Home" });
  await waitFor(() =>
    expect(screen.getByRole("tooltip")).toHaveTextContent(points[0]!.time),
  );
  fireEvent.keyDown(trigger, { key: "ArrowRight" });
  await waitFor(() =>
    expect(screen.getByRole("tooltip")).toHaveTextContent("missing"),
  );
  fireEvent.keyDown(trigger, { key: "End" });
  await waitFor(() =>
    expect(screen.getByRole("tooltip")).toHaveTextContent(points[2]!.time),
  );
  expect(
    view.container
      .querySelector(".argus-mini-trend__line")!
      .getAttribute("d")!
      .match(/M/g),
  ).toHaveLength(2);
  fireEvent.keyDown(trigger, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument(),
  );
});
