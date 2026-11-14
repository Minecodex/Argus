// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useState } from "react";
import { useListMetrics } from "./use-list-metrics";

const client = vi.hoisted(() => ({
  kubernetesWorkbench: { prepareMetrics: vi.fn(), readMetrics: vi.fn() },
}));
vi.mock("@argus/api-client", async (original) => ({
  ...(await original<object>()),
  useApi: () => client,
}));
const caches: QueryClient[] = [];
const Wrapper = ({ children }: { children: ReactNode }) => {
  const [cache] = useState(() => {
    const value = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    caches.push(value);
    return value;
  });
  return <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
};
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
const context = (token = "ctx") => ({
  token,
  observed_at: "2026-11-13T12:00:00Z",
  expires_at: "2026-11-13T12:02:00Z",
  complete: true,
});
const result = (uid = "pod") => ({
  items: [{ uid, trends: [] }],
  observed_at: "2026-11-13T12:00:00Z",
  from: "2026-11-13T11:30:00Z",
  to: "2026-11-13T12:00:00Z",
  complete: true,
});
const options = {
  clusterId: "cluster",
  input: { type: "pod", namespace_mode: "all_authorized" as const },
  uids: ["pod"],
  updatedAt: 1,
  fetching: false,
  scopeKey: "scope-v1",
};
beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(document, "hidden", {
    configurable: true,
    value: false,
  });
});
afterEach(() => {
  cleanup();
  for (const cache of caches.splice(0)) cache.clear();
});

it("prepares once and lets each metric finish independently", async () => {
  const prepared = deferred<ReturnType<typeof context>>(),
    cpu = deferred<ReturnType<typeof result>>(),
    memory = deferred<ReturnType<typeof result>>(),
    storage = deferred<ReturnType<typeof result>>();
  client.kubernetesWorkbench.prepareMetrics.mockReturnValue(prepared.promise);
  client.kubernetesWorkbench.readMetrics.mockImplementation(
    (_id, input) =>
      ({ cpu, memory, storage })[input.metric as "cpu" | "memory" | "storage"]
        .promise,
  );
  const hook = renderHook(() => useListMetrics(options), { wrapper: Wrapper });
  expect(hook.result.current.cpu!.loading).toBe(true);
  expect(client.kubernetesWorkbench.readMetrics).not.toHaveBeenCalled();
  await act(async () => prepared.resolve(context()));
  await waitFor(() =>
    expect(client.kubernetesWorkbench.readMetrics).toHaveBeenCalledTimes(3),
  );
  await act(async () => memory.resolve(result()));
  await waitFor(() =>
    expect(hook.result.current.memory!.data?.items[0]?.uid).toBe("pod"),
  );
  expect(hook.result.current.cpu!.loading).toBe(true);
  await act(async () => storage.reject(new Error("storage failed")));
  await waitFor(() => expect(hook.result.current.storage!.error).toBeTruthy());
  expect(hook.result.current.memory!.data).toBeTruthy();
  await act(async () => cpu.resolve(result()));
  await waitFor(() => expect(hook.result.current.cpu!.loading).toBe(false));
  expect(client.kubernetesWorkbench.prepareMetrics).toHaveBeenCalledTimes(1);
});

it("cancels the previous page and never backfills its late result into another scope", async () => {
  const old = deferred<ReturnType<typeof result>>(),
    signals: AbortSignal[] = [];
  client.kubernetesWorkbench.prepareMetrics.mockImplementation((_id, input) =>
    Promise.resolve(context(input.uids[0])),
  );
  client.kubernetesWorkbench.readMetrics.mockImplementation(
    (_id, input, signal: AbortSignal) => {
      if (input.token === "pod") {
        signals.push(signal);
        return old.promise;
      }
      return Promise.resolve(result("new-pod"));
    },
  );
  const hook = renderHook((props) => useListMetrics(props), {
    wrapper: Wrapper,
    initialProps: options,
  });
  await waitFor(() => expect(signals).toHaveLength(3));
  hook.rerender({ ...options, uids: ["new-pod"], scopeKey: "scope-v2" });
  await waitFor(() =>
    expect(signals.every((signal) => signal.aborted)).toBe(true),
  );
  await waitFor(() =>
    expect(hook.result.current.cpu!.data?.items[0]?.uid).toBe("new-pod"),
  );
  await act(async () => old.resolve(result("pod")));
  expect(hook.result.current.cpu!.data?.items[0]?.uid).toBe("new-pod");
});

it("cancels hidden-page requests and prepares a fresh context on return", async () => {
  const signals: AbortSignal[] = [];
  client.kubernetesWorkbench.prepareMetrics.mockImplementation(() =>
    Promise.resolve(context()),
  );
  client.kubernetesWorkbench.readMetrics.mockImplementation(
    (_id, _input, signal: AbortSignal) => {
      signals.push(signal);
      return new Promise(() => {});
    },
  );
  renderHook(() => useListMetrics(options), { wrapper: Wrapper });
  await waitFor(() => expect(signals).toHaveLength(3));
  act(() => {
    Object.defineProperty(document, "hidden", { value: true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await waitFor(() =>
    expect(signals.slice(0, 3).every((signal) => signal.aborted)).toBe(true),
  );
  act(() => {
    Object.defineProperty(document, "hidden", { value: false });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await waitFor(() =>
    expect(client.kubernetesWorkbench.prepareMetrics).toHaveBeenCalledTimes(2),
  );
});

it("does not enrich Job or CronJob and waits for a stable base list", async () => {
  const hook = renderHook((props) => useListMetrics(props), {
    wrapper: Wrapper,
    initialProps: { ...options, input: { ...options.input, type: "job" } },
  });
  expect(client.kubernetesWorkbench.prepareMetrics).not.toHaveBeenCalled();
  hook.rerender({ ...options, input: { ...options.input, type: "cronjob" } });
  expect(client.kubernetesWorkbench.prepareMetrics).not.toHaveBeenCalled();
  hook.rerender({ ...options, fetching: true });
  expect(client.kubernetesWorkbench.prepareMetrics).not.toHaveBeenCalled();
});
