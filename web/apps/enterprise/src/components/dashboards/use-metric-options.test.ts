import { afterEach, expect, it, vi } from "vitest";
import { createMetricOptionsLoader } from "./use-metric-options";

afterEach(() => vi.useRealTimers());

it("freezes relative time across pages and retries, but refreshes on a new search", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-27T10:00:00Z"));
  const catalog = vi
    .fn()
    .mockResolvedValue({ metrics: [], next_cursor: "page-2" });
  const load = createMetricOptionsLoader(catalog, {
    source_binding: { source_type: "hostmetrics", capability_version: "v1" },
    resource_ids: ["host"],
    time: { kind: "relative", seconds: 3600 },
  });
  const signal = new AbortController().signal;
  await load("cpu", undefined, signal);
  const first = catalog.mock.calls[0]![0];
  vi.advanceTimersByTime(30_000);
  await load("cpu", "page-2", signal);
  expect(catalog.mock.calls[1]![0]).toEqual({ ...first, cursor: "page-2" });
  catalog.mockRejectedValueOnce(new Error("temporary"));
  await expect(load("cpu", "page-3", signal)).rejects.toThrow("temporary");
  vi.advanceTimersByTime(30_000);
  await load("cpu", "page-3", signal);
  expect(catalog.mock.calls[3]![0]).toEqual({ ...first, cursor: "page-3" });
  await load("memory", undefined, signal);
  expect(catalog.mock.calls[4]![0].to).toBe("2026-10-27T10:01:00.000Z");
  expect(catalog.mock.calls[4]![0].search).toBe("memory");
});

it("keeps absolute bounds and prevents aborted results updating metric metadata", async () => {
  const received = vi.fn();
  const descriptor = { name: "cpu", type: "gauge", unit: "1" };
  const catalog = vi.fn().mockResolvedValue({ metrics: [descriptor] });
  const time = {
    kind: "absolute" as const,
    from: "2026-10-27T09:00:00Z",
    to: "2026-10-27T10:00:00Z",
  };
  const load = createMetricOptionsLoader(
    catalog,
    {
      source_binding: { source_type: "hostmetrics", capability_version: "v1" },
      resource_ids: [],
      time,
    },
    received,
  );
  const controller = new AbortController();
  controller.abort();
  await load("cpu", undefined, controller.signal);
  expect(received).not.toHaveBeenCalled();
  expect(catalog.mock.calls[0]![0]).toMatchObject({
    from: "2026-10-27T09:00:00.000Z",
    to: "2026-10-27T10:00:00.000Z",
  });
});
