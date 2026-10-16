import { afterEach, expect, test, vi } from "vitest";
import type { ConversationEvent } from "@argus/api-client/contracts";
import { createConversationRefresh } from "./conversation-refresh";
import { QueryClient, QueryObserver } from "@tanstack/react-query";

afterEach(() => vi.useRealTimers());

test("real query observers stay within two reads per card while replaying a full history", async () => {
  vi.useFakeTimers();
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 15000 } },
  });
  const reads = Array.from({ length: 89 }, () =>
    vi.fn(async () => ({ status: "ready" })),
  );
  const observers = reads.map(
    (queryFn, index) =>
      new QueryObserver(cache, {
        queryKey: ["tool-presentation", "conversation", `call-${index}`],
        queryFn,
      }),
  );
  const unsubscribe = observers.map((observer) => observer.subscribe(() => {}));
  await vi.advanceTimersByTimeAsync(1);
  const refresh = createConversationRefresh(cache, "conversation");
  for (let index = 0; index < 89; index++) refresh.event(event(index));
  await vi.advanceTimersByTimeAsync(51);
  expect(reads.every((read) => read.mock.calls.length === 2)).toBe(true);
  expect(reads.reduce((total, read) => total + read.mock.calls.length, 0)).toBe(
    178,
  );
  refresh.dispose();
  unsubscribe.forEach((stop) => stop());
  cache.clear();
});
function event(index: number, call = `call-${index}`): ConversationEvent {
  return {
    schema_version: "argus.conversation_event/v1",
    event_id: `event-${index}`,
    sequence: index + 1,
    enterprise_id: "enterprise",
    conversation_id: "conversation",
    event_type: "tool_presentation",
    occurred_at: "2026-10-15T00:00:00Z",
    content_hash: "hash",
    payload: { tool_call_id: call },
  };
}
test("89 replayed readiness events refresh their own cards once and the event list once", () => {
  vi.useFakeTimers();
  const invalidateQueries = vi.fn(),
    refresh = createConversationRefresh({ invalidateQueries }, "conversation");
  for (let index = 0; index < 89; index++) refresh.event(event(index));
  expect(invalidateQueries).not.toHaveBeenCalled();
  vi.advanceTimersByTime(50);
  expect(invalidateQueries).toHaveBeenCalledTimes(90);
  for (let index = 0; index < 89; index++)
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["tool-presentation", "conversation", `call-${index}`],
      exact: true,
    });
  refresh.dispose();
});
test("duplicate events and duplicate target cards cannot amplify replay requests", () => {
  vi.useFakeTimers();
  const invalidateQueries = vi.fn(),
    refresh = createConversationRefresh({ invalidateQueries }, "conversation");
  for (let index = 0; index < 50; index++) {
    refresh.event(event(1));
    refresh.event(event(2, "call-1"));
  }
  vi.advanceTimersByTime(50);
  refresh.event(event(1));
  vi.advanceTimersByTime(50);
  expect(invalidateQueries).toHaveBeenCalledTimes(2);
  refresh.dispose();
});
test("live events have bounded latency and subsequent batches still update new data", () => {
  vi.useFakeTimers();
  const invalidateQueries = vi.fn(),
    refresh = createConversationRefresh({ invalidateQueries }, "conversation");
  refresh.event(event(1));
  vi.advanceTimersByTime(49);
  expect(invalidateQueries).not.toHaveBeenCalled();
  vi.advanceTimersByTime(1);
  expect(invalidateQueries).toHaveBeenCalledTimes(2);
  refresh.event(event(2));
  vi.advanceTimersByTime(50);
  expect(invalidateQueries).toHaveBeenCalledTimes(4);
  refresh.dispose();
});
test("unrelated conversations, malformed targets and unmounted subscriptions never invalidate all cards", () => {
  vi.useFakeTimers();
  const invalidateQueries = vi.fn(),
    refresh = createConversationRefresh({ invalidateQueries }, "conversation");
  refresh.event({ ...event(0), conversation_id: "other" });
  refresh.event({ ...event(1), payload: { tool_call_id: ["call"] } });
  vi.advanceTimersByTime(50);
  expect(invalidateQueries).toHaveBeenCalledTimes(1);
  refresh.event(event(2));
  refresh.dispose();
  vi.advanceTimersByTime(50);
  refresh.event(event(3));
  vi.advanceTimersByTime(50);
  expect(invalidateQueries).toHaveBeenCalledTimes(1);
});
