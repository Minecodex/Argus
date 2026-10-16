import { expect, test, vi } from "vitest";
import { createTemplateLoadQueue } from "./load-queue";

test("a long history starts no more than four Runtime boots at once", () => {
  const schedule = createTemplateLoadQueue();
  const releases: (() => void)[] = [];
  let peak = 0,
    active = 0,
    started = 0;
  for (let index = 0; index < 89; index++)
    schedule((release) => {
      started++;
      peak = Math.max(peak, ++active);
      releases.push(() => {
        active--;
        release();
      });
    });
  expect(started).toBe(4);
  while (releases.length) releases.shift()!();
  expect(started).toBe(89);
  expect(peak).toBe(4);
  expect(active).toBe(0);
});

test("unmount cancels a waiting card and frees active startup slots", () => {
  const schedule = createTemplateLoadQueue(1),
    first = vi.fn(),
    removed = vi.fn(),
    last = vi.fn();
  const cancelFirst = schedule(first),
    cancelWaiting = schedule(removed);
  schedule(last);
  cancelWaiting();
  cancelFirst();
  expect(removed).not.toHaveBeenCalled();
  expect(last).toHaveBeenCalledOnce();
  cancelFirst();
  expect(last).toHaveBeenCalledOnce();
});

test("duplicate completion cannot release a different card's slot", () => {
  const schedule = createTemplateLoadQueue(1),
    releases: (() => void)[] = [],
    calls = vi.fn();
  for (let i = 0; i < 3; i++)
    schedule((release) => {
      calls();
      releases.push(release);
    });
  releases[0]!();
  releases[0]!();
  expect(calls).toHaveBeenCalledTimes(2);
  releases[1]!();
  expect(calls).toHaveBeenCalledTimes(3);
});

test("synchronous completion drains a long queue without recursive startup", () => {
  const schedule = createTemplateLoadQueue(1),
    releases: (() => void)[] = [];
  schedule((release) => releases.push(release));
  let calls = 0;
  for (let i = 0; i < 10000; i++)
    schedule((release) => {
      calls++;
      release();
    });
  releases[0]!();
  expect(calls).toBe(10000);
});
