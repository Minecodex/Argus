// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "./locale";
import { ObservationFreshness } from "./observation-freshness";
afterEach(cleanup);
it.each(["zh-CN", "en-US"])(
  "separates query time, original sample time, cache and unknown ingestion (%s)",
  (locale) => {
    localStorage.setItem("argus.locale", locale);
    render(
      <LocaleProvider>
        <ObservationFreshness
          targets={[
            {
              id: "A",
              meta: {
                query_completed_at: "2026-09-25T10:00:00Z",
                latest_sample_at: "2026-09-25T09:00:00Z",
                sample_time_basis: "observed_query_samples",
                cache_hit: true,
              },
            },
          ]}
        />
      </LocaleProvider>,
    );
    const name =
      locale === "zh-CN" ? "数据时间与完整性" : "Data time and completeness";
    fireEvent.click(screen.getByRole("button", { name }));
    const container = screen.getByRole("dialog", { name });
    expect(
      [...container.querySelectorAll("time")].map((t) => t.dateTime),
    ).toEqual(["2026-09-25T10:00:00Z", "2026-09-25T09:00:00Z"]);
    expect(screen.getByRole("dialog", { name })).toBeInTheDocument();
    expect(container).toHaveTextContent(
      locale === "zh-CN"
        ? "摄入完整性: 未知"
        : "Ingestion completeness: Unknown",
    );
    expect(container).toHaveTextContent(
      locale === "zh-CN" ? "缓存结果" : "Cached result",
    );
  },
);
it("does not invent latest event time from an unproven timestamp", () => {
  localStorage.setItem("argus.locale", "en-US");
  render(
    <LocaleProvider>
      <ObservationFreshness
        targets={[
          {
            id: "A",
            meta: {
              latest_sample_at: "2026-09-25T10:00:00Z",
              sample_time_basis: "unknown",
            },
          },
        ]}
      />
    </LocaleProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Data time and completeness" }),
  );
  expect(screen.getByRole("dialog").querySelectorAll("time")).toHaveLength(0);
});
