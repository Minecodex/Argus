import { afterEach, expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";

afterEach(() => vi.unstubAllGlobals());
it("preserves audit facts and forwards filters together with the signed page cursor", async () => {
  const fetcher = vi.fn<typeof fetch>(
    async () =>
      new Response(
        JSON.stringify({
          items: [
            {
              id: "00000000-0000-4000-8000-000000000001",
              domain: "enterprise",
              actor_type: "enterprise_user",
              actor_id: "editor",
              action: "dashboard.published",
              result: "success",
              resource_type: "dashboard",
              resource_id: "board",
              resource_display_name: "Board",
              details: {
                draft_version: 4,
                revision_id: "revision",
                spec_hash: "digest",
              },
              previous_hash: "a".repeat(64),
              event_hash: "b".repeat(64),
              created_at: "2026-09-27T00:00:00Z",
            },
          ],
          page: {
            next_cursor: "next",
            has_more: true,
            partial: { partial: false, reasons: [] },
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
  );
  vi.stubGlobal("fetch", fetcher);
  const api = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
  });
  const result = await api.audit.list(
    {
      action: "dashboard.published",
      actorUserId: "editor",
      resourceType: "dashboard",
      resourceId: "board",
      result: "success",
      query: "Board",
      from: "2026-09-26T00:00:00Z",
      to: "2026-09-28T00:00:00Z",
    },
    { page: { cursor: "cursor", limit: 2 } },
  );
  const url = new URL(String(fetcher.mock.calls[0]![0]));
  expect(Object.fromEntries(url.searchParams)).toEqual({
    action: "dashboard.published",
    actor_id: "editor",
    resource_type: "dashboard",
    resource_id: "board",
    result: "success",
    query: "Board",
    from: "2026-09-26T00:00:00Z",
    to: "2026-09-28T00:00:00Z",
    cursor: "cursor",
    limit: "2",
  });
  expect(result.items[0]?.details).toEqual({
    draft_version: 4,
    revision_id: "revision",
    spec_hash: "digest",
  });
  expect(result.items[0]?.eventHash).toBe("b".repeat(64));
  expect(result.nextCursor).toBe("next");
});
