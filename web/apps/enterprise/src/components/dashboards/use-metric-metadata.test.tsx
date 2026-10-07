// @vitest-environment jsdom
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ApiProvider,
  createMockApiClient,
  type DashboardSchemas,
} from "@argus/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { newTarget } from "./model";
import { useMetricMetadata } from "./use-metric-metadata";
import { metricOperationCompatible, selectMetric } from "./metric-metadata";
afterEach(cleanup);
it("reconciles async metadata while ignoring a late descriptor from the previous metric", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 }),
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const original = api.dashboards.catalog;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await api.auth.login({ username: "root", password: "123456" });
  vi.spyOn(api.dashboards, "catalog").mockImplementation(
    async (input, signal) => {
      if (input.metric === "system_cpu_utilization") await gate;
      return original(input, signal);
    },
  );
  const panel = {
    signal: "metrics" as const,
    source_binding: { source_type: "otlp", capability_version: "v1" },
  };
  const target = newTarget("metrics", "stat", "builder"),
    change = vi.fn();
  const { result, rerender } = renderHook(
    ({ metric }) =>
      useMetricMetadata(
        panel,
        {
          ...target,
          source_definition: {
            builder: { ...target.source_definition.builder!, metric },
          },
        },
        { time: { kind: "relative", seconds: 3600 }, resources: [] },
        change,
      ),
    {
      initialProps: { metric: "system_cpu_utilization" },
      wrapper: ({ children }: { children: ReactNode }) => (
        <ApiProvider client={api}>
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        </ApiProvider>
      ),
    },
  );
  rerender({ metric: "http_requests_total" });
  await waitFor(() => expect(result.current.metadata?.type).toBe("counter"));
  await waitFor(() => expect(change).toHaveBeenCalled());
  release();
  await waitFor(() => expect(result.current.catalog.isFetching).toBe(false));
  expect(
    change.mock.calls.every(
      ([t]) =>
        t.source_definition.builder.metric === "http_requests_total" &&
        t.source_definition.builder.metric_type === "counter",
    ),
  ).toBe(true);
  client.clear();
});
it("never inherits another metric's type or treats an incompatible operation as compatible", () => {
  const builder: DashboardSchemas["DashboardBuilder"] = {
    operation: "rate",
    metric: "counter",
    metric_type: "counter",
    filters: [],
    group_by: [],
  };
  expect(selectMetric(builder, "unknown").metric_type).toBeUndefined();
  expect(builder.metric_type).toBe("counter");
  expect(
    metricOperationCompatible("rate", {
      name: "cpu",
      type: "gauge",
      unit: "1",
      labels: [],
    }),
  ).toBe(false);
  expect(
    metricOperationCompatible("p95", {
      name: "latency",
      type: "histogram",
      unit: "s",
      labels: [],
    }),
  ).toBe(true);
  expect(metricOperationCompatible("rate")).toBe(true);
});
