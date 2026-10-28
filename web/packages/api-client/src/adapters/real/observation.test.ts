import { it, expect, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";

it("carries typed scopes and AbortSignal through all query adapters", async () => {
  const calls: { url: string; input: RequestInit }[] = [];
  const fetch = vi.fn(
    async (url: string | URL | Request, input?: RequestInit) => {
      calls.push({ url: String(url), input: input ?? {} });
      return new Response("{}", {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  const api = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
    fetch,
  });
  const controller = new AbortController(),
    scope = {
      mode: "explicit" as const,
      resources: [
        {
          type: "application" as const,
          id: "4aab1d68-fffa-4b72-8e7e-5ebebd0408d5",
        },
      ],
      source_type: "otlp",
    };
  const input = {
    scope,
    query: "example",
    time_range: { from: "2026-10-28T00:00:00Z", to: "2026-10-28T01:00:00Z" },
  };
  await api.telemetry.queryMetrics(input, controller.signal);
  await api.telemetry.queryMetricsRange(
    { ...input, step_seconds: 30 },
    controller.signal,
  );
  await api.telemetry.queryLogs(input, controller.signal);
  await api.telemetry.queryTraces(input, controller.signal);
  expect(calls).toHaveLength(4);
  for (const call of calls) {
    expect(call.input.signal).toBe(controller.signal);
    expect(JSON.parse(String(call.input.body))).toMatchObject({ scope });
    expect(JSON.parse(String(call.input.body))).not.toHaveProperty(
      "resource_ids",
    );
  }
});
it("uses the resource catalog instead of the dashboard catalog", async () => {
  const urls: string[] = [];
  const fetch = vi.fn(async (url: string | URL | Request) => {
    urls.push(String(url));
    return new Response("{}", {
      headers: { "Content-Type": "application/json" },
    });
  });
  const api = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
    fetch,
  });
  await api.telemetry.resources({
    kind: "host",
    search: "edge",
    cursor: "signed",
    limit: 30,
  });
  await api.telemetry.catalog({
    scope: { mode: "explicit", resources: [] },
    signal: "metrics",
    kind: "metrics",
    from: "2026-10-28T00:00:00Z",
    to: "2026-10-28T01:00:00Z",
    limit: 30,
  });
  expect(urls[0]).toContain("/enterprise/telemetry/query/resources?");
  expect(urls[1]).toContain("/enterprise/telemetry/query/catalog");
});
