import { expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";

it("sends disabled collection filters explicitly and omits an unrestricted enablement filter", async () => {
  const urls: URL[] = [];
  const fetch = vi.fn(async (input: string | URL | Request) => {
    urls.push(new URL(String(input)));
    return new Response(JSON.stringify({ items: [] }), {
      headers: { "Content-Type": "application/json" },
    });
  });
  const api = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
    fetch,
  });
  await api.monitoring.collections({
    desired_enabled: false,
    integration: "mysql",
    apply_status: "effective",
    health: "healthy",
    freshness: "stale",
    search: "orders",
    limit: 2,
    cursor: "signed-cursor",
  });
  expect(Object.fromEntries(urls[0]!.searchParams)).toEqual({
    desired_enabled: "false",
    integration: "mysql",
    apply_status: "effective",
    health: "healthy",
    freshness: "stale",
    search: "orders",
    limit: "2",
    cursor: "signed-cursor",
  });
  await api.monitoring.collections({ desired_enabled: undefined });
  expect(urls[1]!.searchParams.has("desired_enabled")).toBe(false);
});
