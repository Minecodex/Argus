import { expect, it } from "vitest";
import { freezeObservationTime } from "./time-range";

it("keeps historical absolute boundaries and derives the actual duration", () => {
  expect(
    freezeObservationTime(
      {
        kind: "absolute",
        from: "2026-10-28T08:00:00+08:00",
        to: "2026-10-28T09:30:00+08:00",
      },
      new Date("2030-01-01"),
    ),
  ).toEqual({
    range: { from: "2026-10-28T00:00:00.000Z", to: "2026-10-28T01:30:00.000Z" },
    seconds: 5400,
  });
});
it("freezes a relative window once and rejects invalid or reversed ranges", () => {
  expect(
    freezeObservationTime(
      { kind: "relative", seconds: 900 },
      new Date("2026-10-28T01:00:00Z"),
    ).range,
  ).toEqual({
    from: "2026-10-28T00:45:00.000Z",
    to: "2026-10-28T01:00:00.000Z",
  });
  for (const value of [
    { kind: "absolute" as const, from: "bad", to: "2026-10-28" },
    { kind: "relative" as const, seconds: 0 },
    { kind: "absolute" as const, from: "2026-10-28", to: "2026-10-27" },
  ])
    expect(() => freezeObservationTime(value)).toThrow(
      "OBSERVE_TIME_RANGE_INVALID",
    );
});
