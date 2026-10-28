// @vitest-environment jsdom
import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { makeQuery } from "./model";
import { useObserveWorkspace } from "./use-workspace";
const api = vi.hoisted(() => ({
  telemetry: {
    queryMetricsRange: vi.fn<
      (input: unknown, signal: AbortSignal) => Promise<unknown>
    >(async () => ({
      data: { resultType: "matrix", result: [] },
      argus_meta: { partial: false, elapsed_ms: 1 },
    })),
    queryLogs: vi.fn<(input: unknown, signal: AbortSignal) => Promise<unknown>>(
      async () => ({
        result_type: "logs",
        data: [],
        meta: { elapsed_ms: 1 },
      }),
    ),
    queryTraces: vi.fn<
      (input: unknown, signal: AbortSignal) => Promise<unknown>
    >(async () => ({
      data: { queryTraces: { traces: [] } },
      extensions: { argus: { partial: false, elapsed_ms: 1 } },
    })),
  },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@argus/api-client", async (original) => ({
  ...(await original<object>()),
  useApi: () => api,
}));
it("sends one exact historical absolute window to all three signals and derives range resolution from it", async () => {
  const queries = (["metrics", "logs", "traces"] as const).map((signal) => {
    const query = makeQuery(signal, {
      mode: "explicit",
      resources: [{ type: "host", id: "host-a" }],
      source_type: "hostmetrics",
    });
    query.panel.authoring_mode = "dsl";
    query.panel.targets[0]!.source_definition = {
      dsl: { expression: "query" },
    };
    return query;
  });
  const { result, unmount } = renderHook(() =>
    useObserveWorkspace(queries[0]!),
  );
  act(() => {
    result.current.add(queries[1]!);
    result.current.add(queries[2]!);
  });
  const selection = {
    kind: "absolute" as const,
    from: "2025-06-01T08:00:00+08:00",
    to: "2025-06-01T09:30:00+08:00",
  };
  act(() =>
    result.current.run(
      queries.map((q) => q.id),
      selection,
    ),
  );
  await waitFor(() =>
    expect(result.current.queries.every((q) => q.status === "success")).toBe(
      true,
    ),
  );
  const window = {
    from: "2025-06-01T00:00:00.000Z",
    to: "2025-06-01T01:30:00.000Z",
  };
  for (const method of [
    api.telemetry.queryMetricsRange,
    api.telemetry.queryLogs,
    api.telemetry.queryTraces,
  ])
    expect(method.mock.calls[0]?.[0]).toMatchObject({ time_range: window });
  expect(api.telemetry.queryMetricsRange.mock.calls[0]?.[0]).toMatchObject({
    step_seconds: 18,
  });
  act(() => result.current.invalidateAll());
  expect(result.current.queries.every((q) => !q.result)).toBe(true);
  unmount();
});
