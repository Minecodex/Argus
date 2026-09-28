export type ObservationSeries = {
  name: string;
  labels: Record<string, string>;
  points: Array<[number, number | null]>;
  row: Record<string, unknown>;
};
export const observationObject = (
  value: unknown,
): Record<string, unknown> | undefined =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
export function metricObservations(value: unknown): ObservationSeries[] {
  const root = observationObject(value);
  const rows = Array.isArray(value)
    ? value
    : Array.isArray(root?.result)
      ? root.result
      : [];
  if (rows.length === 2 && typeof rows[0] === "number")
    return [
      {
        name: "value",
        labels: {},
        points: [[Number(rows[0]) * 1000, finite(rows[1])]],
        row: { value: rows },
      },
    ];
  return rows.flatMap((entry, index) => {
    const row = observationObject(entry);
    if (!row) return [];
    const labels = Object.fromEntries(
      Object.entries(observationObject(row.metric) ?? {}).filter(
        (p): p is [string, string] => typeof p[1] === "string",
      ),
    );
    const raw = Array.isArray(row.values)
      ? row.values
      : Array.isArray(row.value)
        ? [row.value]
        : [];
    const points = raw.flatMap((point): Array<[number, number | null]> =>
      Array.isArray(point) && Number.isFinite(Number(point[0]))
        ? [[Number(point[0]) * 1000, finite(point[1])]]
        : [],
    );
    const dimensions = Object.entries(labels)
      .filter(([key]) => key !== "__name__" && !key.startsWith("argus_"))
      .map(([key, v]) => `${key}=${v}`)
      .join(", ");
    return [
      {
        name: dimensions || labels.__name__ || String(index + 1),
        labels,
        points,
        row,
      },
    ];
  });
}
function finite(value: unknown): number | null {
  if (value === null || value === undefined || value === "") return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}
export function lastObservation(series: ObservationSeries): number | null {
  return series.points.at(-1)?.[1] ?? null;
}
// State intervals end at the next observed sample; gaps and the end of the query are not extrapolated.
export function stateIntervals(
  series: ObservationSeries,
): Array<[number, number, number]> {
  const result: Array<[number, number, number]> = [];
  for (let i = 0; i < series.points.length; i++) {
    const [start, value] = series.points[i]!;
    if (value === null) continue;
    const end = series.points[i + 1]?.[0] ?? start,
      previous = result.at(-1);
    if (previous && previous[1] === start && previous[2] === value)
      previous[1] = end;
    else result.push([start, end, value]);
  }
  return result;
}
export function observationRows(value: unknown): Record<string, unknown>[] {
  if (Array.isArray(value))
    return value.flatMap((v) =>
      observationObject(v) ? [v as Record<string, unknown>] : [],
    );
  const root = observationObject(value);
  if (!root) return [];
  for (const key of ["rows", "traces", "spans"])
    if (Array.isArray(root[key])) return observationRows(root[key]);
  for (const child of Object.values(root)) {
    const rows = observationRows(child);
    if (rows.length) return rows;
  }
  return [];
}
export function logAggregateObservations(value: unknown): ObservationSeries[] {
  const groups = new Map<string, ObservationSeries>();
  for (const row of observationRows(value)) {
    if (!("count" in row) && !("value" in row)) continue;
    const name = String(
      row.group_value ?? ("count" in row ? "count" : "value"),
    );
    const group = groups.get(name) ?? {
      name,
      labels: row.group_value === undefined ? {} : { group_value: name },
      points: [],
      row,
    };
    const time =
      row.timestamp === undefined ? 0 : Date.parse(String(row.timestamp));
    if (Number.isFinite(time))
      group.points.push([time, finite(row.count ?? row.value)]);
    groups.set(name, group);
  }
  return [...groups.values()].map((group) => ({
    ...group,
    points: group.points.sort((a, b) => a[0] - b[0]),
  }));
}
export function observationEnvelope(
  value: unknown,
): Record<string, unknown> | undefined {
  const root = observationObject(value);
  if (!root) return undefined;
  if (
    "basis" in root ||
    "completeness" in root ||
    "nodes" in root ||
    "spans" in root
  )
    return root;
  for (const child of Object.values(root)) {
    const result = observationEnvelope(child);
    if (result) return result;
  }
  return undefined;
}
export function histogramObservations(
  series: ObservationSeries[],
): { names: string[]; values: number[] } | null {
  if (!series.length || series.some((s) => s.labels.le === undefined))
    return null;
  // Different label identities must not be silently aggregated.
  const identity = (s: ObservationSeries) =>
    JSON.stringify(
      Object.entries(s.labels)
        .filter(([k]) => k !== "le")
        .sort(),
    );
  if (series.some((s) => identity(s) !== identity(series[0]!))) return null;
  const boundary = (s: ObservationSeries) =>
    s.labels.le === "+Inf" ? Infinity : Number(s.labels.le);
  if (
    series.some(
      (s) =>
        s.labels.le!.trim() === "" ||
        (!Number.isFinite(boundary(s)) && s.labels.le !== "+Inf"),
    )
  )
    return null;
  if (new Set(series.map(boundary)).size !== series.length) return null;
  // Cumulative buckets must describe one snapshot, not different evaluation times.
  const timestamp = series[0]!.points.at(-1)?.[0];
  if (series.some((s) => s.points.at(-1)?.[0] !== timestamp)) return null;
  const ordered = [...series].sort(
    (a, b) =>
      Number(a.labels.le === "+Inf" ? Infinity : a.labels.le) -
      Number(b.labels.le === "+Inf" ? Infinity : b.labels.le),
  );
  let previous = 0;
  const names: string[] = [],
    values: number[] = [];
  for (const item of ordered) {
    const value = lastObservation(item);
    if (value === null || value < previous) return null;
    names.push(item.labels.le!);
    values.push(value - previous);
    previous = value;
  }
  return { names, values };
}
export function formatObservation(
  value: number | null,
  unit = "",
  decimals = 2,
): string {
  if (value === null || !Number.isFinite(value)) return "—";
  const places = Math.max(0, Math.min(8, decimals));
  if (unit === "percent_ratio") return `${(value * 100).toFixed(places)}%`;
  if (unit === "bytes") {
    const power = Math.max(
      0,
      Math.min(4, Math.floor(Math.log2(Math.max(1, Math.abs(value))) / 10)),
    );
    return `${(value / 1024 ** power).toFixed(places)} ${["B", "KiB", "MiB", "GiB", "TiB"][power]}`;
  }
  return `${value.toFixed(places)}${unit === "percent" ? "%" : unit ? ` ${unit}` : ""}`;
}
