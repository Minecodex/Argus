import type { PromQLRangeQuery } from "../generated/contracts";

/** Deterministic samples for explicit mock mode; the real adapter always uses HTTP. */
export function hostListTrendSamples(input: PromQLRangeQuery, ids: string[]) {
  const from = new Date(input.time_range.from).getTime() / 1000;
  const to = new Date(input.time_range.to).getTime() / 1000;
  const cpu = input.query.includes("system_cpu_utilization");
  const storage = input.query.includes("system_filesystem_usage");
  return ids.flatMap((id, index) =>
    ["used", "capacity", "observed"].map((component) => {
      const capacity = cpu ? 1 : (storage ? 80 : 8) * 1024 ** 3;
      const values: [number, string][] = [];
      for (let at = from; at <= to; at += input.step_seconds) {
        const value =
          component === "observed"
            ? at - 5
            : component === "capacity"
              ? capacity
              : capacity *
                (0.3 + index / 100 + 0.05 * Math.sin((at - from) / 180));
        values.push([at, String(value)]);
      }
      return {
        metric: {
          argus_resource_id: id,
          argus_host_component: component,
          ...(storage
            ? { device: "/dev/mock", mountpoint: "/", type: "ext4" }
            : {}),
        },
        values,
      };
    }),
  );
}
