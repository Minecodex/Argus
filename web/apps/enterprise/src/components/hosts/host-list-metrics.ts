import type { PrometheusQueryResponse } from "@argus/api-client";

export type HostMetricKind = "cpu" | "memory" | "storage";
export const hostMetricKinds = ["cpu", "memory", "storage"] as const;
export const HOST_TREND_SECONDS = 1800;
export const HOST_TREND_STEP = 30;
export type HostMetricState = "ready" | "no_data" | "stale" | "partial";
export type HostMetricPoint = {
  time: string;
  used: number | null;
  capacity: number | null;
  sampledAt: number | null;
  value: number | null;
};
export type HostMetricTrend = {
  state: HostMetricState;
  basis: "capacity" | "usage";
  points: HostMetricPoint[];
  filesystem?: string;
};
export type HostMetricGroup = {
  loading: boolean;
  denied?: boolean;
  error?: unknown;
  data?: Record<string, HostMetricTrend>;
};

const host = "argus_resource_id";
const component = (query: string, name: string) =>
  `label_replace((${query}), "argus_host_component", "${name}", "", "")`;
const idle = 'system_cpu_utilization{state="idle"}';
const fsLabels = `${host},device,mountpoint,type`;
const expressions = {
  cpu: {
    // hostmetrics normally exports a host-wide average, not one series per core.
    // One here denotes the full utilization fraction, never one CPU core.
    used: `1 - avg by (${host}) (${idle})`,
    capacity: `0 * avg by (${host}) (${idle}) + 1`,
    observed: `max by (${host}) (timestamp(${idle}))`,
  },
  memory: {
    used: `sum by (${host}) (system_memory_usage{state="used"})`,
    capacity: `sum by (${host}) (system_memory_usage)`,
    observed: `max by (${host}) (timestamp(system_memory_usage{state="used"}))`,
  },
  storage: {
    used: `max by (${fsLabels}) (system_filesystem_usage{state="used"})`,
    capacity: `sum by (${fsLabels}) (system_filesystem_usage)`,
    observed: `max by (${fsLabels}) (timestamp(system_filesystem_usage{state="used"}))`,
  },
};

/** Fixed queries; resource authorization and source resolution stay on the server. */
export function hostMetricExpression(metric: HostMetricKind) {
  return Object.entries(expressions[metric])
    .map(([name, query]) => component(query, name))
    .join(" or ");
}

type Series = { metric: Record<string, string>; values: [number, string][] };
type Samples = Record<string, Map<number, number>>;

function finite(value: unknown): number | undefined {
  if (typeof value !== "string" && typeof value !== "number") return undefined;
  if (value === "") return undefined;
  const n = Number(value);
  return Number.isFinite(n) && n >= 0 ? n : undefined;
}

/** Join by exact host, filesystem and evaluation time; never borrow another row's denominator. */
export function parseHostMetricResponse(
  response: PrometheusQueryResponse,
  ids: string[],
  metric: HostMetricKind,
  from: number,
  to: number,
): Record<string, HostMetricTrend> {
  if (
    response.data.resultType !== "matrix" ||
    !Array.isArray(response.data.result)
  ) {
    throw new Error("QUERY_RESULT_INVALID");
  }
  const groups = new Map<
    string,
    Map<string, { samples: Samples; name?: string }>
  >();
  const allowed = new Set(ids);
  for (const raw of response.data.result as Series[]) {
    if (!raw?.metric || !Array.isArray(raw.values))
      throw new Error("QUERY_RESULT_INVALID");
    const id = raw.metric.argus_resource_id || raw.metric.resource_id;
    if (!id || !allowed.has(id)) continue;
    const name = raw.metric.argus_host_component;
    if (!name || !["used", "capacity", "observed"].includes(name)) continue;
    const key =
      metric === "storage"
        ? JSON.stringify([
            raw.metric.device,
            raw.metric.mountpoint,
            raw.metric.type,
          ])
        : "host";
    if (!groups.has(id)) groups.set(id, new Map());
    const rows = groups.get(id)!;
    if (!rows.has(key))
      rows.set(key, {
        samples: {},
        name:
          metric === "storage"
            ? [raw.metric.mountpoint, raw.metric.device, raw.metric.type]
                .filter(Boolean)
                .join(" · ")
            : undefined,
      });
    const series = rows.get(key)!;
    if (!series.samples[name]) series.samples[name] = new Map();
    for (const pair of raw.values) {
      if (!Array.isArray(pair) || pair.length !== 2)
        throw new Error("QUERY_RESULT_INVALID");
      const stamp = Number(pair[0]),
        value = finite(pair[1]);
      if (
        Number.isFinite(stamp) &&
        stamp >= from &&
        stamp <= to &&
        value !== undefined
      ) {
        series.samples[name]!.set(stamp, value);
      }
    }
  }
  return Object.fromEntries(
    ids.map((id) => {
      if (response.argus_meta.partial)
        return [id, { state: "partial", basis: "usage", points: [] }];
      const candidates = [...(groups.get(id)?.values() ?? [])].map(
        ({ samples, name }) => {
          const points: HostMetricPoint[] = [];
          for (let at = from; at <= to; at += HOST_TREND_STEP) {
            points.push({
              time: new Date(at * 1000).toISOString(),
              used: samples.used?.get(at) ?? null,
              capacity: samples.capacity?.get(at) ?? null,
              sampledAt: samples.observed?.get(at) ?? null,
              value: null,
            });
          }
          const latest = points.filter((point) => point.used !== null).at(-1);
          const basis =
            latest?.capacity != null && latest.capacity > 0
              ? ("capacity" as const)
              : ("usage" as const);
          for (const point of points)
            point.value =
              basis === "capacity"
                ? point.used != null &&
                  point.capacity != null &&
                  point.capacity > 0
                  ? (100 * point.used) / point.capacity
                  : null
                : point.used;
          // A filesystem is selected once for the entire window, so the line never switches disks mid-way.
          const score = latest?.value ?? -1;
          const sampledAt = latest?.sampledAt ?? null;
          const state: HostMetricState =
            latest == null
              ? "no_data"
              : sampledAt == null || to - sampledAt > 300
                ? "stale"
                : "ready";
          return { state, basis, points, filesystem: name, score };
        },
      );
      candidates.sort(
        (a, b) =>
          Number(b.state === "ready") - Number(a.state === "ready") ||
          Number(b.basis === "capacity") - Number(a.basis === "capacity") ||
          b.score - a.score ||
          (a.filesystem ?? "").localeCompare(b.filesystem ?? ""),
      );
      return [
        id,
        candidates[0] ?? { state: "no_data", basis: "usage", points: [] },
      ];
    }),
  );
}
