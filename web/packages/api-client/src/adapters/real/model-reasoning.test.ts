import { expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";

it("round-trips thinking depth and explicitly resets provider defaults without leaking keys", async () => {
  const model = (revision: number, effort?: "medium") => ({
    id: "11111111-1111-4111-8111-111111111111",
    name: "Reasoning model",
    base_url: "https://models.example.test/v1",
    model_id: "reasoning-model",
    api_protocol: "chat_completions",
    context_window_tokens: 131072,
    max_output_tokens: 8192,
    input_price_per_million: 1,
    output_price_per_million: 2,
    capabilities: {
      supports_tool_calling: true,
      supports_structured_output: true,
      ...(effort ? { reasoning_effort: effort } : {}),
    },
    status: "enabled",
    health_status: "healthy",
    revision,
    version: revision,
    created_at: "2026-11-02T00:00:00Z",
    updated_at: "2026-11-02T00:00:00Z",
  });
  const json = (data: unknown) =>
    new Response(JSON.stringify(data), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      json({ compatible: true, model: model(1, "medium"), checks: [] }),
    )
    .mockResolvedValueOnce(json(model(2, "medium")))
    .mockResolvedValueOnce(json(model(3)));
  const api = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
    csrf_token: () => "csrf",
    fetch,
  });
  const created = await api.models.testAndCreate({
    name: "Reasoning model",
    baseUrl: "https://models.example.test/v1",
    apiKey: "fixture-only-key",
    modelId: "reasoning-model",
    apiProtocol: "chat_completions",
    reasoningEffort: "medium",
    contextWindowTokens: 131072,
    maxOutputTokens: 8192,
    inputPricePerMillionTokens: 1,
    outputPricePerMillionTokens: 2,
  });
  expect(created.model?.reasoningEffort).toBe("medium");
  expect(created.model).not.toHaveProperty("apiKey");
  expect(
    JSON.parse(String(fetch.mock.calls[0]?.[1]?.body)).reasoning_effort,
  ).toBe("medium");
  const renamed = await api.models.update(created.model!.id, {
    name: "Renamed",
  });
  expect(renamed.reasoningEffort).toBe("medium");
  expect(JSON.parse(String(fetch.mock.calls[1]?.[1]?.body))).not.toHaveProperty(
    "reasoning_effort",
  );
  const reset = await api.models.update(created.model!.id, {
    reasoningEffort: "provider_default",
  });
  expect(reset.reasoningEffort).toBe("provider_default");
  expect(JSON.parse(String(fetch.mock.calls[2]?.[1]?.body))).toMatchObject({
    reasoning_effort: "provider_default",
    expected_version: 2,
  });
});
