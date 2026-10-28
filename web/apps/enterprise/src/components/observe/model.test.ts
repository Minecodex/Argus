import { describe, it, expect, vi } from "vitest";
import { makeQuery, ObserveScheduler, switchQuerySignal } from "./model";

describe("temporary observation queries", () => {
  it("starts with explicit empty resources and a metric definition the real compiler accepts", () => {
    const q = makeQuery();
    expect(q.scope).toEqual({ mode: "explicit", resources: [] });
    expect(q.result).toBeUndefined();
    expect(q.panel.targets[0]!.source_definition.builder).toMatchObject({
      metric: "",
      operation: "value",
    });
    expect(
      q.panel.targets[0]!.source_definition.builder?.limit,
    ).toBeUndefined();
  });
  it("keeps each language draft while preserving the panel's current resource selection", () => {
    const q = makeQuery();
    q.scope = {
      mode: "explicit",
      resources: [{ type: "host", id: "host-a" }],
      source_type: "hostmetrics",
    };
    q.panel.authoring_mode = "dsl";
    q.panel.targets[0]!.source_definition = { dsl: { expression: "cpu + 1" } };
    const logs = switchQuerySignal(q, "logs");
    logs.scope.resources = [{ type: "host", id: "host-b" }];
    const back = switchQuerySignal(logs, "metrics");
    expect(back.panel.targets[0]!.source_definition.dsl?.expression).toBe(
      "cpu + 1",
    );
    expect(back.scope.resources[0]?.id).toBe("host-b");
    expect(back.id).toBe(q.id);
  });
  it("limits executions to three across repeated scheduling and cancellation", async () => {
    const scheduler = new ObserveScheduler(3),
      started: string[] = [],
      finish = new Map<string, () => void>();
    let active = 0,
      peak = 0;
    const work = (name: string) => (signal: AbortSignal) =>
      new Promise<void>((resolve) => {
        started.push(name);
        active++;
        peak = Math.max(peak, active);
        const end = () => {
          active--;
          resolve();
        };
        finish.set(name, end);
        signal.addEventListener("abort", end, { once: true });
      });
    for (const id of ["a", "b", "c", "d", "e"])
      scheduler.schedule(id, work(id));
    expect(started).toEqual(["a", "b", "c"]);
    scheduler.cancel("d");
    finish.get("a")!();
    await vi.waitFor(() => expect(started).toEqual(["a", "b", "c", "e"]));
    expect(peak).toBe(3);
    scheduler.cancelAll();
  });
  it("waits for an aborted in-flight request to settle before reusing the execution slot", async () => {
    const scheduler = new ObserveScheduler(1);
    let settle!: () => void;
    let signal: AbortSignal | undefined;
    scheduler.schedule("a", (s) => {
      signal = s;
      return new Promise<void>((resolve) => {
        settle = resolve;
      });
    });
    const next = vi.fn(async () => {});
    scheduler.schedule("a", next);
    expect(signal?.aborted).toBe(true);
    expect(next).not.toHaveBeenCalled();
    settle();
    await vi.waitFor(() => expect(next).toHaveBeenCalledOnce());
    scheduler.cancelAll();
  });
  it("clears queued requests when leaving the page", async () => {
    const scheduler = new ObserveScheduler(1),
      queued = vi.fn(async () => {});
    let settle!: () => void;
    scheduler.schedule(
      "a",
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        }),
    );
    scheduler.schedule("b", queued);
    scheduler.cancelAll();
    settle();
    await Promise.resolve();
    await Promise.resolve();
    expect(queued).not.toHaveBeenCalled();
  });
});
