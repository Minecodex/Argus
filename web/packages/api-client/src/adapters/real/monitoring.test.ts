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
  const targetId = "fb8cc9bd-d700-4876-98d4-ff877cf75376";
  await api.monitoring.collections({
    target_id: targetId,
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
    target_id: targetId,
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
  expect(urls[1]!.searchParams.has("target_id")).toBe(false);
  await api.monitoring.applications({
    environment: "production",
    search: "Orders",
    limit: 1,
    cursor: "signed-cursor",
  });
  expect(Object.fromEntries(urls[2]!.searchParams)).toEqual({
    environment: "production",
    search: "Orders",
    limit: "1",
    cursor: "signed-cursor",
  });
  await api.monitoring.applications();
  expect(urls[3]!.searchParams.has("environment")).toBe(false);
});
