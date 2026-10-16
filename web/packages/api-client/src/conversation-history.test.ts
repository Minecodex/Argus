import { expect, it, vi } from "vitest";
import type { ArgusApiClient } from "./client";
import type { RealDomainContext } from "./adapters/real/context";
import { installAgentDomains } from "./adapters/real/agent";
function clientWith(request: ReturnType<typeof vi.fn>) {
  const client = {} as ArgusApiClient;
  installAgentDomains({
    client,
    http: { request },
    sse: {},
    versions: new Map(),
    remember: (v: unknown) => v,
    expectedVersion: () => 1,
    idempotencyKey: () => "key",
  } as unknown as RealDomainContext);
  return client;
}
it("loads every ledger page so long conversations retain every presentation", async () => {
  const events = Array.from({ length: 403 }, (_, index) => ({
    sequence: index + 1,
    event_id: String(index + 1),
  }));
  const request = vi.fn(async (url: string) => {
    const cursor = Number(
      new URL(url, "https://example.test").searchParams.get("cursor") ?? 0,
    );
    return { items: events.slice(cursor, cursor + 200) };
  });
  expect(
    await clientWith(request).conversations.listEvents("conversation"),
  ).toHaveLength(403);
  expect(request).toHaveBeenCalledTimes(3);
  expect(request.mock.calls[2]![0]).toContain("cursor=400");
});
it("rejects non-progressing ledger pages instead of looping or silently truncating", async () => {
  const rows = Array.from({ length: 200 }, (_, index) => ({
    sequence: index + 1,
    event_id: String(index + 1),
  }));
  const request = vi.fn(async () => ({ items: rows }));
  await expect(
    clientWith(request).conversations.listEvents("conversation"),
  ).rejects.toThrow("CONVERSATION_LEDGER_CURSOR_INVALID");
  expect(request).toHaveBeenCalledTimes(2);
});
