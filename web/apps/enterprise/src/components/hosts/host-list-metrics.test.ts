import { describe, expect, it } from "vitest";
import type { PrometheusQueryResponse } from "@argus/api-client";
import { parseHostMetricResponse } from "./host-list-metrics";

const to = 1800000000,
  from = to - 60;
function response(
  rows: {
    id: string;
    component: string;
    values: [number, string][];
    mount?: string;
  }[],
  partial = false,
) {
  return {
    data: {
      resultType: "matrix",
      result: rows.map((row) => ({
        metric: {
          argus_resource_id: row.id,
          argus_host_component: row.component,
          ...(row.mount
            ? { device: "/dev/test", mountpoint: row.mount, type: "ext4" }
            : {}),
        },
        values: row.values,
      })),
    },
    argus_meta: { partial },
  } as PrometheusQueryResponse;
}
const samples = (
  id: string,
  used: number,
  capacity: number,
  mount?: string,
) => [
  {
    id,
    component: "used",
    values: [
      [from, String(used)],
      [to, String(used)],
    ] as [number, string][],
    mount,
  },
  {
    id,
    component: "capacity",
    values: [
      [from, String(capacity)],
      [to, String(capacity)],
    ] as [number, string][],
    mount,
  },
  {
    id,
    component: "observed",
    values: [
      [from, String(from - 5)],
      [to, String(to - 5)],
    ] as [number, string][],
    mount,
  },
];

describe("Host list metric semantics", () => {
  it("keeps exact resource identities, same-time denominators, zeroes and missing points", () => {
    const parsed = parseHostMetricResponse(
      response([
        ...samples("a", 0, 100),
        ...samples("b", 50, 200),
        ...samples("outside", 99, 100),
      ]),
      ["a", "b", "empty"],
      "memory",
      from,
      to,
    );
    expect(Object.keys(parsed)).toEqual(["a", "b", "empty"]);
    expect(parsed.a!.points.map((point) => point.value)).toEqual([0, null, 0]);
    expect(parsed.b!.points.at(-1)?.value).toBe(25);
    expect(parsed.a!.state).toBe("ready");
    expect(parsed.empty!.state).toBe("no_data");
  });
  it("does not turn an incomplete aggregate into an exact rate", () => {
    const partial = parseHostMetricResponse(
      response(samples("a", 1, 2), true),
      ["a"],
      "memory",
      from,
      to,
    );
    expect(partial.a!.state).toBe("partial");
    expect(partial.a!.points).toEqual([]);
  });
  it("uses raw usage if capacity is absent and never substitutes a current capacity into history", () => {
    const rows = samples("a", 20, 100);
    rows[1]!.values = [[to, "100"]];
    const parsed = parseHostMetricResponse(
      response(rows),
      ["a"],
      "memory",
      from,
      to,
    ).a!;
    expect(parsed.points[0]!.value).toBeNull();
    expect(parsed.points.at(-1)!.value).toBe(20);
    const missing = parseHostMetricResponse(
      response(rows.filter((row) => row.component !== "capacity")),
      ["a"],
      "memory",
      from,
      to,
    ).a!;
    expect(missing.basis).toBe("usage");
    expect(missing.points.at(-1)!.used).toBe(20);
  });
  it("picks one filesystem for its whole history instead of adding duplicate mounts or switching disks", () => {
    const rows = [
      ...samples("a", 300, 1000, "/bind-one"),
      ...samples("a", 300, 1000, "/bind-two"),
      ...samples("a", 70, 100, "/data"),
    ];
    const parsed = parseHostMetricResponse(
      response(rows),
      ["a"],
      "storage",
      from,
      to,
    ).a!;
    expect(parsed.filesystem).toContain("/data");
    expect(parsed.points.at(-1)!.value).toBe(70);
    expect(parsed.points[0]!.used).toBe(70);
  });
  it("uses actual sample time to detect staleness, not the evaluation timestamp", () => {
    const rows = samples("a", 0.2, 1);
    rows[2]!.values = [[to, String(to - 301)]];
    expect(
      parseHostMetricResponse(response(rows), ["a"], "cpu", from, to).a!.state,
    ).toBe("stale");
  });
  it("rejects malformed matrix responses and does not plot NaN as zero", () => {
    expect(() =>
      parseHostMetricResponse(
        {
          data: { resultType: "vector", result: [] },
        } as unknown as PrometheusQueryResponse,
        ["a"],
        "cpu",
        from,
        to,
      ),
    ).toThrow();
    const rows = samples("a", 1, 2);
    rows[0]!.values = [[to, "NaN"]];
    expect(
      parseHostMetricResponse(response(rows), ["a"], "memory", from, to).a!
        .state,
    ).toBe("no_data");
  });
});
