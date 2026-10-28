import type { DashboardSchemas } from "@argus/api-client";

export type ObservationTimeRange = DashboardSchemas["DashboardTimeRange"];

/** One snapshot per execution batch, including relative ranges. */
export function freezeObservationTime(
  value: ObservationTimeRange,
  now = new Date(),
) {
  const to =
    value.kind === "absolute" ? Date.parse(value.to ?? "") : now.getTime();
  const from =
    value.kind === "absolute"
      ? Date.parse(value.from ?? "")
      : to - (value.seconds ?? 3600) * 1000;
  if (!Number.isFinite(from) || !Number.isFinite(to) || from >= to)
    throw new Error("OBSERVE_TIME_RANGE_INVALID");
  return {
    range: {
      from: new Date(from).toISOString(),
      to: new Date(to).toISOString(),
    },
    seconds: (to - from) / 1000,
  };
}
