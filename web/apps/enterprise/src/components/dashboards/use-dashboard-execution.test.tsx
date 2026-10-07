// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  emptyDashboardSpec,
  type DashboardExecution,
  type DashboardRevision,
  type DashboardSchemas,
} from "@argus/api-client";
import { newPanel } from "./model";
import { dependentPanels } from "./execution-dependencies";
import { useDashboardExecution } from "./use-dashboard-execution";

const all = { all: true, values: [] };
const prod = { all: false, values: ["production"] };
function fixture() {
  const spec = emptyDashboardSpec();
  spec.default_refresh_seconds = 5;
  spec.panels = ["dependent", "cascade", "independent"].map((id) => ({
    ...newPanel(spec, id),
    id,
  }));
  spec.variables = ["environment", "service", "unused"].map((name) => ({
    id: name,
    name,
    label: name,
    multiple: false,
    include_all: true,
    default: all,
    query: {
      signal: "metrics",
      source_binding: spec.panels[0]!.source_binding,
      metric: "requests",
      field: name,
      filters:
        name === "service"
          ? [{ field: "environment", operator: "=", variable: "environment" }]
          : [],
    },
  }));
  spec.panels[0]!.targets[0]!.source_definition.builder!.filters = [
    { field: "environment", operator: "=", variable: "environment" },
  ];
  spec.panels[1]!.targets[0]!.source_definition.builder!.filters = [
    { field: "service", operator: "=", variable: "service" },
  ];
  const revision: DashboardRevision = {
    id: "r1",
    dashboard_id: "board",
    revision_number: 1,
    name: "board",
    description: "",
    spec,
    spec_hash: "hash",
    validation: { valid: true, issues: [], compiler_version: "v3" },
    sample: {},
    created_at: new Date().toISOString(),
  };
  const pending: {
    input: DashboardSchemas["DashboardExecutionInput"];
    signal: AbortSignal;
    resolve: (result: DashboardExecution) => void;
    reject: (error: Error) => void;
  }[] = [];
  const execute = vi.fn(
    (
      _id: string,
      input: DashboardSchemas["DashboardExecutionInput"],
      signal: AbortSignal,
    ) =>
      new Promise<DashboardExecution>((resolve, reject) =>
        pending.push({ input, signal, resolve, reject }),
      ),
  );
  const options = {
    dashboardId: "board",
    revision,
    enabled: true,
    execute,
    onRevisionChanged: vi.fn(async () => {}),
    onReset: vi.fn(),
    formatError: (e: unknown) => String(e),
  };
  const hook = renderHook((props) => useDashboardExecution(props), {
    initialProps: options,
  });
  async function finish(
    index: number,
    overrides: Partial<DashboardExecution> = {},
  ) {
    const request = pending[index]!;
    const result: DashboardExecution = {
      dashboard_id: "board",
      revision_id: "r1",
      execution_id: `e${index}`,
      execution_hash: `h${index}`,
      context_token: `token${index}`,
      context_expires_at: new Date().toISOString(),
      from: request.input.from!,
      to: request.input.to!,
      resources: [],
      partial: false,
      variables: request.input.variables ?? {},
      local_values: request.input.local_values ?? {},
      variable_candidates: {},
      local_candidates: {},
      panels: spec.panels
        .filter(
          (p) =>
            !request.input.candidates_only &&
            (!request.input.panel_ids?.length ||
              request.input.panel_ids.includes(p.id)),
        )
        .map((p) => ({
          id: p.id,
          status: "success",
          sources: [],
          detail_sources: {},
          targets: [
            {
              id: "a",
              status: "success",
              data: index,
              query_hash: "hash",
              result_type: "matrix",
              meta: {},
            },
          ],
        })),
      ...overrides,
    };
    await act(async () => request.resolve(result));
    return result;
  }
  return { ...hook, spec, options, pending, execute, finish };
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

it("does not cancel slow queries on auto refresh, and pauses scheduling while hidden", async () => {
  const f = fixture();
  await act(async () => vi.advanceTimersByTimeAsync(16000));
  expect(f.execute).toHaveBeenCalledTimes(1);
  expect(f.pending[0]!.signal.aborted).toBe(false);
  await f.finish(0);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  act(() => document.dispatchEvent(new Event("visibilitychange")));
  await act(async () => vi.advanceTimersByTimeAsync(60000));
  expect(f.execute).toHaveBeenCalledTimes(1);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  act(() => document.dispatchEvent(new Event("visibilitychange")));
  await act(async () => vi.advanceTimersByTimeAsync(5000));
  expect(f.execute).toHaveBeenCalledTimes(2);
});

it("refreshes transitive dependencies in the same interval and preserves unrelated data and drilldown tokens", async () => {
  const f = fixture();
  const candidate = {
    status: "success" as const,
    values: ["production"],
    complete: true,
    selected_exists: { production: true },
    reset: false,
    sources: [],
  };
  await f.finish(0, {
    local_candidates: { independent: { environment: candidate } },
  });
  const original = f.result.current.execution!.panels[2];
  await act(async () => vi.advanceTimersByTimeAsync(1000));
  act(() => f.result.current.applyVariable("environment", prod));
  expect(f.pending[1]!.input.panel_ids).toEqual(["dependent", "cascade"]);
  expect(f.pending[1]!.input.from).toBe(f.pending[0]!.input.from);
  expect(f.pending[1]!.input.to).toBe(f.pending[0]!.input.to);
  await f.finish(1, {
    local_candidates: { independent: {}, dependent: {}, cascade: {} },
  });
  expect(f.result.current.execution!.panels[2]).toBe(original);
  expect(
    f.result.current.execution!.local_candidates.independent?.environment,
  ).toEqual(candidate);
  expect(f.result.current.tokens.current.get("independent")).toBe("token0");
  expect(f.result.current.tokens.current.get("dependent")).toBe("token1");
});

it("reconciles unused variables without executing any panel", async () => {
  const f = fixture();
  await f.finish(0);
  const previous = f.result.current.execution!.panels;
  act(() => f.result.current.applyVariable("unused", prod));
  expect(f.pending[1]!.input.candidates_only).toBe(true);
  await f.finish(1);
  expect(f.result.current.execution!.panels).toEqual(previous);
});

it("preserves candidate failure coverage across partial refreshes and clears it after recovery", async () => {
  const f = fixture();
  const unavailable = {
    status: "unavailable" as const,
    values: [],
    complete: false,
    selected_exists: {},
    reset: false,
    sources: [],
  };
  await f.finish(0, {
    partial: true,
    variable_candidates: { environment: unavailable },
    local_candidates: { independent: { term: unavailable } },
  });
  expect(f.result.current.execution!.partial).toBe(true);
  act(() => f.result.current.applyVariable("service", prod));
  await f.finish(1); // This does not revalidate the unrelated local candidate.
  expect(f.result.current.execution!.partial).toBe(true);
  act(() =>
    f.result.current.apply(f.result.current.latest.current, ["independent"]),
  );
  await f.finish(2, { local_candidates: { independent: {} } });
  expect(f.result.current.execution!.partial).toBe(false);
  act(() => f.result.current.applyVariable("unused", prod));
  await f.finish(3, {
    variable_candidates: {
      unused: { ...unavailable, status: "skipped_budget" },
    },
    partial: true,
  });
  expect(f.result.current.execution!.partial).toBe(true);
});

it("propagates a proven fallback from candidate-only refresh to dependent panels", async () => {
  const f = fixture();
  await f.finish(0, {
    variables: { environment: prod, service: all, unused: all },
  });
  act(() => f.result.current.applyVariable("unused", prod));
  await f.finish(1, {
    variables: { environment: all, service: all, unused: prod },
  });
  expect(f.pending[2]!.input.panel_ids).toEqual(["dependent", "cascade"]);
  expect(f.pending[2]!.input.to).toBe(f.pending[0]!.input.to);
  await f.finish(2);
  expect(f.result.current.input.variables?.environment).toEqual(all);
  expect(f.result.current.tokens.current.get("independent")).toBe("token0");
});

it("unions pending partial changes, cancels stale responses, and retains both selections", async () => {
  const f = fixture();
  await f.finish(0);
  act(() => f.result.current.applyVariable("service", prod));
  act(() =>
    f.result.current.apply(
      {
        ...f.result.current.latest.current,
        local_values: { independent: { term: prod } },
      },
      ["independent"],
    ),
  );
  expect(f.pending[1]!.signal.aborted).toBe(true);
  expect(f.pending[2]!.input.panel_ids).toEqual(["independent", "cascade"]);
  expect(f.pending[2]!.input.variables?.service).toEqual(prod);
  await f.finish(2);
  await f.finish(1);
  expect(f.result.current.execution!.panels[1]!.targets[0]!.data).toBe(2);
  expect(f.result.current.input.local_values?.independent?.term).toEqual(prod);
  expect(f.result.current.busy).toBe(false);
});

it("keeps condition changes made during the initial full query and ignores its late response", async () => {
  const f = fixture();
  const time = f.pending[0]!.input.to;
  act(() => f.result.current.applyVariable("environment", prod));
  expect(f.pending[0]!.signal.aborted).toBe(true);
  expect(f.pending[1]!.input.panel_ids).toBeUndefined();
  expect(f.pending[1]!.input.to).toBe(time);
  await f.finish(1);
  await f.finish(0);
  expect(f.result.current.input.variables?.environment).toEqual(prod);
  expect(f.result.current.execution!.panels[2]!.targets[0]!.data).toBe(1);
});

it("cancels a query when time changes, rejects revision drift, and aborts on history/unmount", async () => {
  const f = fixture();
  act(() => {
    void f.result.current.run(f.result.current.latest.current, undefined, {
      kind: "relative",
      seconds: 60,
    });
  });
  expect(f.pending[0]!.signal.aborted).toBe(true);
  expect(
    new Date(f.pending[1]!.input.to!).getTime() -
      new Date(f.pending[1]!.input.from!).getTime(),
  ).toBe(60000);
  await f.finish(1, { revision_id: "r2" });
  expect(f.options.onRevisionChanged).toHaveBeenCalledTimes(1);
  expect(f.result.current.execution).toBeUndefined();
  act(() => {
    void f.result.current.run(f.result.current.latest.current);
  });
  f.rerender({ ...f.options, enabled: false });
  expect(f.pending[2]!.signal.aborted).toBe(true);
  await f.finish(2);
  expect(f.result.current.execution).toBeUndefined();
  f.rerender(f.options);
  f.unmount();
  expect(f.pending[3]!.signal.aborted).toBe(true);
});

it("keeps viewer refresh overrides temporary and reschedules the timer", async () => {
  const f = fixture();
  await f.finish(0);
  expect(f.result.current.refreshSeconds).toBe(5);
  act(() => f.result.current.setRefreshSeconds(0));
  await act(async () => vi.advanceTimersByTimeAsync(30000));
  expect(f.execute).toHaveBeenCalledTimes(1);
  act(() => f.result.current.setRefreshSeconds(10));
  await act(async () => vi.advanceTimersByTimeAsync(9999));
  expect(f.execute).toHaveBeenCalledTimes(1);
  await act(async () => vi.advanceTimersByTimeAsync(1));
  expect(f.execute).toHaveBeenCalledTimes(2);
  expect(f.pending[1]!.input.panel_ids).toBeUndefined();
  expect(f.spec.default_refresh_seconds).toBe(5);
});

it("retains selection on candidate failure instead of inventing an All fallback", async () => {
  const f = fixture();
  await f.finish(0);
  act(() => f.result.current.applyVariable("unused", prod));
  await act(async () => f.pending[1]!.reject(new Error("unavailable")));
  expect(f.result.current.input.variables?.unused).toEqual(prod);
  expect(f.result.current.error).toContain("unavailable");
  expect(f.execute).toHaveBeenCalledTimes(2);
});

it("recognizes local candidate dependencies, ratio error filters and statement detail bindings", () => {
  const f = fixture();
  const panel = f.spec.panels[2]!;
  panel.targets[0]!.source_definition.builder!.error_filters = [
    { field: "environment", operator: "=", variable: "unused" },
  ];
  expect(dependentPanels(f.spec, ["unused"])).toEqual(["independent"]);
  panel.targets[0]!.source_definition = { dsl: { expression: "requests" } };
  panel.targets[0]!.parameter_bindings = [
    { parameter: "p", variable: "unused" },
  ];
  panel.detail_query_targets = panel.targets;
  panel.targets = [];
  expect(dependentPanels(f.spec, ["unused"])).toEqual(["independent"]);
  panel.detail_query_targets = [];
  panel.local_filters = [
    {
      id: "instance",
      label: "instance",
      kind: "query",
      multiple: false,
      required: false,
      default: all,
      query: {
        ...f.spec.variables[1]!.query,
        filters: [{ field: "service", operator: "=", variable: "service" }],
      },
    },
  ];
  expect(dependentPanels(f.spec, ["environment"])).toEqual([
    "dependent",
    "cascade",
    "independent",
  ]);
});
