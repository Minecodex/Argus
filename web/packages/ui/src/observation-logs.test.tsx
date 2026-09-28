// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "./locale";
import { ObservationPanel } from "./observation-panel";

afterEach(cleanup);
it.each(["zh-CN", "en-US"])(
  "keeps log bodies readable and passes the exact published row to drilldown (%s)",
  (locale) => {
    localStorage.setItem("argus.locale", locale);
    const select = vi.fn();
    const rows = Array.from({ length: 52 }, (_, i) => ({
      timestamp: "2026-09-27T10:00:00Z",
      severity_text: "ERROR",
      body: `line ${i}\nstructured body stays intact`,
      event_id: `event-${i}`,
      source_id: "source",
      resource_id: "resource",
      trace_id: "trace",
      structured_metadata: {
        probe: i === 51 ? "hidden-search-match" : "normal",
      },
    }));
    const { container } = render(
      <LocaleProvider>
        <ObservationPanel
          title="Logs"
          type="logs"
          signal="logs"
          targets={[
            {
              id: "published",
              status: "success",
              result_type: "log_entries",
              data: rows,
            },
          ]}
          onSelect={select}
        />
      </LocaleProvider>,
    );
    expect(container.querySelector("table")).toBeNull();
    expect(screen.getAllByRole("article")).toHaveLength(50);
    const first = screen.getAllByRole("article")[0]!;
    expect(
      first.querySelector(".argus-observation-log-body")!.textContent,
    ).toBe(rows[0]!.body);
    fireEvent.click(
      within(first).getByRole("button", {
        name: locale === "zh-CN" ? "关联查询" : "Related queries",
      }),
    );
    expect(select).toHaveBeenCalledWith(rows[0], "published");
    fireEvent.click(
      screen.getByRole("button", {
        name: locale === "zh-CN" ? "下一页" : "Next",
      }),
    );
    expect(screen.getAllByRole("article")).toHaveLength(2);
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "hidden-search-match" },
    });
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(
      screen
        .getAllByRole("article")[0]!
        .querySelector(".argus-observation-log-body")!.textContent,
    ).toBe(rows[51]!.body);
    expect(
      screen.queryByRole("button", { name: /下载|Download|Export/ }),
    ).toBeNull();
  },
);
