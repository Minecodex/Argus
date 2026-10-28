import {
  emptyDashboardSpec,
  type DashboardPanel,
  type ObservationScope,
  type ObservationSignal,
  type ObservationMetric,
} from "@argus/api-client";
import { newPanel, newTarget } from "../dashboards/model";
export type QueryResult = {
  result_type: string;
  data: unknown;
  warnings?: string[];
  partial?: boolean;
  elapsed_ms?: number;
};
export type ObserveQuery = {
  id: string;
  panel: DashboardPanel;
  scope: ObservationScope;
  revision: number;
  view: "graph" | "table";
  status:
    | "idle"
    | "dirty"
    | "queued"
    | "running"
    | "success"
    | "empty"
    | "error"
    | "cancelled";
  result?: QueryResult;
  error?: string;
  conversionError?: string;
  compiled?: string;
  range?: { from: string; to: string };
  drafts: Partial<
    Record<
      ObservationSignal,
      { panel: DashboardPanel; source_type?: string; source_keys?: string[] }
    >
  >;
};
export function makeQuery(
  signal: ObservationSignal = "metrics",
  scope: ObservationScope = { mode: "explicit", resources: [] },
): ObserveQuery {
  const spec = emptyDashboardSpec(),
    panel = newPanel(spec, "");
  panel.signal = signal;
  panel.type =
    signal === "metrics"
      ? "timeseries"
      : signal === "logs"
        ? "logs"
        : "trace_list";
  panel.authoring_mode = "builder";
  panel.source_binding = {
    source_type: scope.source_type ?? "",
    capability_version: "v1",
  };
  const target = newTarget(signal, panel.type, "builder");
  if (target.source_definition.builder) {
    target.source_definition.builder.metric =
      signal === "metrics" ? "" : undefined;
    target.source_definition.builder.limit =
      signal === "metrics" ? undefined : 200;
  }
  panel.targets = [target];
  return {
    id: panel.id,
    panel,
    scope: structuredClone(scope),
    revision: 0,
    view: "graph",
    status: "idle",
    drafts: {},
  };
}
export const hasQueryResources = (scope: ObservationScope) =>
  scope.mode === "all_authorized" || scope.resources.length > 0;
export function sameResourceSelection(
  a: ObservationScope,
  b: ObservationScope,
) {
  return (
    a.mode === b.mode &&
    [...(a.resource_types ?? [])].sort().join("|") ===
      [...(b.resource_types ?? [])].sort().join("|") &&
    a.resources
      .map((r) => `${r.type}:${r.id}`)
      .sort()
      .join("|") ===
      b.resources
        .map((r) => `${r.type}:${r.id}`)
        .sort()
        .join("|")
  );
}
export function changeQueryScope(
  q: ObserveQuery,
  scope: ObservationScope,
): ObserveQuery {
  const reset = (panel: DashboardPanel): DashboardPanel => ({
    ...panel,
    source_binding: { source_type: "", capability_version: "v1" },
    targets: panel.targets.map((target) =>
      target.source_definition.builder
        ? {
            ...target,
            source_definition: {
              builder: {
                ...target.source_definition.builder,
                metric_type: undefined,
              },
            },
          }
        : target,
    ),
  });
  return {
    ...q,
    scope: structuredClone(scope),
    panel: reset(q.panel),
    drafts: Object.fromEntries(
      Object.entries(q.drafts).map(([signal, value]) => [
        signal,
        { panel: reset(value!.panel) },
      ]),
    ),
  };
}
export function queryFromMetric(
  metric: ObservationMetric,
  scope: ObservationScope,
  expression?: string,
) {
  const q = makeQuery("metrics", {
    ...scope,
    source_type: metric.source_type,
    ...(metric.source_keys.length ? { source_keys: metric.source_keys } : {}),
  });
  q.panel.title = metric.name;
  q.panel.unit = metric.unit;
  const target = q.panel.targets[0]!;
  if (expression) {
    q.panel.authoring_mode = "dsl";
    target.source_definition = { dsl: { expression } };
  } else {
    target.source_definition.builder!.metric = metric.name;
    target.source_definition.builder!.metric_type = [
      "gauge",
      "counter",
      "histogram",
    ].includes(metric.type)
      ? (metric.type as "gauge")
      : undefined;
  }
  return q;
}
export function switchQuerySignal(
  q: ObserveQuery,
  signal: ObservationSignal,
): ObserveQuery {
  const drafts = {
    ...q.drafts,
    [q.panel.signal]: {
      panel: structuredClone(q.panel),
      source_type: q.scope.source_type,
      source_keys: q.scope.source_keys,
    },
  };
  const previous = drafts[signal];
  const fresh = makeQuery(signal, {
    mode: q.scope.mode,
    resources: q.scope.resources,
  });
  return {
    ...q,
    panel: previous
      ? { ...structuredClone(previous.panel), id: q.id }
      : { ...fresh.panel, id: q.id },
    scope: {
      mode: q.scope.mode,
      resources: q.scope.resources,
      source_type: previous?.source_type,
      source_keys: previous?.source_keys,
    },
    drafts,
  };
}

// One page owns the scheduler. Cancelling a running fetch retains its slot until
// the underlying promise settles; repeated Run/Run all cannot exceed the limit.
export class ObserveScheduler {
  private queue: {
    id: string;
    controller: AbortController;
    run: (signal: AbortSignal) => Promise<void>;
  }[] = [];
  private running = new Set<{ id: string; controller: AbortController }>();
  constructor(private readonly concurrency = 3) {}
  cancel(id: string) {
    this.queue = this.queue.filter((task) => {
      if (task.id !== id) return true;
      task.controller.abort();
      return false;
    });
    for (const task of this.running)
      if (task.id === id) task.controller.abort();
  }
  cancelAll() {
    for (const task of [...this.queue, ...this.running])
      task.controller.abort();
    this.queue = [];
  }
  schedule(id: string, run: (signal: AbortSignal) => Promise<void>) {
    this.cancel(id);
    this.queue.push({ id, controller: new AbortController(), run });
    this.pump();
  }
  private pump() {
    while (this.running.size < this.concurrency && this.queue.length) {
      const task = this.queue.shift()!;
      this.running.add(task);
      void task
        .run(task.controller.signal)
        .catch(() => {})
        .finally(() => {
          this.running.delete(task);
          this.pump();
        });
    }
  }
}

export class ObservationConversionError extends Error {}
