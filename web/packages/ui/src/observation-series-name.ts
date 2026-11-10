import type { ObservationSeries } from "./observation-data";

const preferred = [
  "name",
  "queue",
  "controller",
  "namespace",
  "resource_k8s_namespace_name",
  "pod",
  "k8s_pod_name",
  "resource_k8s_pod_name",
  "container",
  "resource_k8s_container_name",
  "node",
  "resource_k8s_node_name",
  "service",
  "resource_service_name",
];
const opaque = (key: string) => /(?:^|_)(?:uid|uuid|id)$/.test(key);
const rank = (key: string) => {
  const index = preferred.indexOf(key);
  return index < 0
    ? preferred.length + (key === "job" || key === "instance" ? 1 : 0)
    : index;
};
const identity = (series: ObservationSeries & { target?: string }) =>
  JSON.stringify([
    series.target ?? "",
    Object.entries(series.labels).sort(([a], [b]) => a.localeCompare(b)),
  ]);
function shortIdentity(value: string) {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i++)
    hash = Math.imul(hash ^ value.charCodeAt(i), 16777619);
  return (hash >>> 0).toString(36);
}

// Presentation labels never replace the original label set or join series.
// Duplicate readable names retain separate legend identities, including when
// only a Pod UID, hidden source identity, or query target distinguishes them.
export function uniqueObservationNames<
  T extends ObservationSeries & { target?: string },
>(series: T[]): T[] {
  const counts = new Map<string, number>();
  for (const item of series)
    counts.set(item.name, (counts.get(item.name) ?? 0) + 1);
  const used = new Set(
    series.filter((s) => counts.get(s.name) === 1).map((s) => s.name),
  );
  return series.map((item) => {
    if (counts.get(item.name) === 1) return item;
    const base = `${item.name} · ${shortIdentity(identity(item))}`;
    let name = base,
      index = 2;
    while (used.has(name)) name = `${base} · ${index++}`;
    used.add(name);
    return { ...item, name };
  });
}

export function labelMetricSeries<T extends ObservationSeries>(
  series: T[],
  isDimension: (key: string) => boolean,
): T[] {
  const keys = [...new Set(series.flatMap((s) => Object.keys(s.labels)))]
    .filter((key) => isDimension(key) && !opaque(key))
    .sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
  const varying = keys.filter(
    (key) => new Set(series.map((s) => s.labels[key] ?? "")).size > 1,
  );
  const context = keys.filter((key) => preferred.includes(key));
  const selected = varying.length ? varying : context.length ? context : keys;
  return uniqueObservationNames(
    series.map((item) => {
      if (typeof item.row.display_name === "string")
        return { ...item, name: item.row.display_name };
      const values = [
        ...new Set(selected.map((key) => item.labels[key]).filter(Boolean)),
      ].slice(0, 3);
      return {
        ...item,
        name: values.join(" · ") || item.labels.__name__ || item.name,
      };
    }),
  );
}
