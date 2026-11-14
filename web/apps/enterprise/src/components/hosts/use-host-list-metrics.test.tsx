// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type {
  Host,
  PromQLRangeQuery,
  PrometheusQueryResponse,
} from "@argus/api-client";
import { useHostListMetrics } from "./use-host-list-metrics";

const mock = vi.hoisted(() => ({ query: vi.fn(), allowed: true }));
vi.mock("@argus/api-client", async (original) => ({
  ...(await original<object>()),
  useApi: () => ({ telemetry: { queryMetricsRange: mock.query } }),
}));
vi.mock("../../lib/permissions", () => ({
  usePermission: (permission: string) =>
    permission === "host.read" && mock.allowed,
}));
const caches: QueryClient[] = [];
function Wrapper({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const cache = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    caches.push(cache);
    return cache;
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function response(input: PromQLRangeQuery): PrometheusQueryResponse {
  const to = new Date(input.time_range.to).getTime() / 1000;
  return {
    data: {
      resultType: "matrix",
      result: input
        .scope!.resources.map((row) =>
          ["used", "capacity", "observed"].map((component) => ({
            metric: {
              argus_resource_id: row.id,
              argus_host_component: component,
            },
            values: [
              [
                to,
                String(
                  component === "observed"
                    ? to - 5
                    : component === "capacity"
                      ? 100
                      : 20,
                ),
              ],
            ],
          })),
        )
        .flat(),
    },
    argus_meta: { partial: false },
  } as PrometheusQueryResponse;
}
const host = (id: string) => ({ id }) as Host;
const options = {
  hosts: [host("a"), host("b")],
  ready: true,
  refreshKey: 0,
  scopeKey: "all",
};
beforeEach(() => {
  vi.clearAllMocks();
  mock.allowed = true;
  Object.defineProperty(document, "hidden", {
    configurable: true,
    value: false,
  });
});
afterEach(() => {
  cleanup();
  caches.splice(0).forEach((cache) => cache.clear());
});

it("batches all visible hosts in three independent queries without waiting for CPU", async () => {
  const cpu = deferred<PrometheusQueryResponse>();
  mock.query.mockImplementation((input: PromQLRangeQuery) =>
    input.query.includes("system_cpu_")
      ? cpu.promise
      : Promise.resolve(response(input)),
  );
  const hook = renderHook(() => useHostListMetrics(options), {
    wrapper: Wrapper,
  });
  await waitFor(() =>
    expect(hook.result.current.memory.data?.a?.state).toBe("ready"),
  );
  expect(hook.result.current.cpu.loading).toBe(true);
  expect(mock.query).toHaveBeenCalledTimes(3);
  for (const [input] of mock.query.mock.calls) {
    expect(input.scope.resources.map((row: { id: string }) => row.id)).toEqual([
      "a",
      "b",
    ]);
    expect(input.scope.source_type).toBe("hostmetrics");
  }
  const input = mock.query.mock.calls.find(([input]) =>
    input.query.includes("system_cpu_"),
  )![0];
  await act(async () => cpu.resolve(response(input)));
  await waitFor(() => expect(hook.result.current.cpu.loading).toBe(false));
});

it("cancels a previous filter and never backfills its result into another host", async () => {
  const old = deferred<PrometheusQueryResponse>(),
    signals: AbortSignal[] = [];
  mock.query.mockImplementation(
    (input: PromQLRangeQuery, signal: AbortSignal) => {
      if (input.scope!.resources[0]!.id === "a") {
        signals.push(signal);
        return old.promise;
      }
      return Promise.resolve(response(input));
    },
  );
  const hook = renderHook((props) => useHostListMetrics(props), {
    wrapper: Wrapper,
    initialProps: options,
  });
  await waitFor(() => expect(signals).toHaveLength(3));
  const initial = mock.query.mock.calls[0]![0];
  hook.rerender({ ...options, hosts: [host("new")], scopeKey: "filtered" });
  await waitFor(() =>
    expect(signals.every((signal) => signal.aborted)).toBe(true),
  );
  await waitFor(() => expect(hook.result.current.cpu.data?.new).toBeTruthy());
  await act(async () => old.resolve(response(initial)));
  expect(hook.result.current.cpu.data?.a).toBeUndefined();
});

it("isolates metric failures and refreshes on demand", async () => {
  mock.query.mockImplementation((input: PromQLRangeQuery) =>
    input.query.includes("system_filesystem_")
      ? Promise.reject(new Error("storage failed"))
      : Promise.resolve(response(input)),
  );
  const hook = renderHook((props) => useHostListMetrics(props), {
    wrapper: Wrapper,
    initialProps: options,
  });
  await waitFor(() => expect(hook.result.current.storage.error).toBeTruthy());
  expect(hook.result.current.memory.data?.a).toBeTruthy();
  hook.rerender({ ...options, refreshKey: 1 });
  await waitFor(() => expect(mock.query).toHaveBeenCalledTimes(6));
});

it("waits for the base list, skips empty lists and hides revoked metrics", async () => {
  mock.query.mockImplementation((input: PromQLRangeQuery) =>
    Promise.resolve(response(input)),
  );
  const hook = renderHook((props) => useHostListMetrics(props), {
    wrapper: Wrapper,
    initialProps: { ...options, ready: false },
  });
  expect(mock.query).not.toHaveBeenCalled();
  hook.rerender({ ...options, hosts: [] });
  expect(mock.query).not.toHaveBeenCalled();
  hook.rerender(options);
  await waitFor(() => expect(hook.result.current.cpu.data?.a).toBeTruthy());
  mock.allowed = false;
  hook.rerender(options);
  expect(hook.result.current.cpu.denied).toBe(true);
  expect(hook.result.current.cpu.data).toBeUndefined();
});

it("cancels while hidden and performs three fresh reads when visible again", async () => {
  const signals: AbortSignal[] = [];
  mock.query.mockImplementation((_input, signal: AbortSignal) => {
    signals.push(signal);
    return new Promise(() => {});
  });
  renderHook(() => useHostListMetrics(options), { wrapper: Wrapper });
  await waitFor(() => expect(signals).toHaveLength(3));
  act(() => {
    Object.defineProperty(document, "hidden", { value: true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await waitFor(() =>
    expect(signals.every((signal) => signal.aborted)).toBe(true),
  );
  act(() => {
    Object.defineProperty(document, "hidden", { value: false });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await waitFor(() => expect(signals).toHaveLength(6));
});
