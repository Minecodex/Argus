import { afterEach, expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";
afterEach(() => vi.unstubAllGlobals());
it("uses the generated platform aggregate contract and preserves recorded usage without list pagination or synthetic data", async () => {
  const body = {
    sampled_at: "2026-10-07T00:00:00Z",
    enterprise_count: 405,
    active_enterprise_count: 403,
    active_sandbox_session_count: 307,
    pending_admin_count: 4,
    usage_from_month: "2025-11",
    usage_to_month: "2026-11",
    monthly_usage: [
      { month: "2026-10", session_count: 615, session_seconds: 6355 },
    ],
  };
  const fetcher = vi.fn<typeof fetch>(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetcher);
  const api = await createConfiguredApiClient({
    portal: "platform",
    mode: "real",
    base_url: "https://api.example.test",
  });
  const controller = new AbortController();
  expect(await api.platform.overview.get(controller.signal)).toEqual(body);
  expect(String(fetcher.mock.calls[0]![0])).toMatch(/\/platform\/overview$/);
  expect(fetcher.mock.calls[0]![1]?.signal).toBe(controller.signal);
  fetcher.mockImplementation(
    async () =>
      new Response(
        JSON.stringify({
          code: "DEPENDENCY_UNAVAILABLE",
          message_key: "errors.unavailable",
          request_id: "overview-test",
          retryable: true,
        }),
        { status: 503, headers: { "Content-Type": "application/json" } },
      ),
  );
  await expect(api.platform.overview.get()).rejects.toMatchObject({
    status: 503,
  });
});
