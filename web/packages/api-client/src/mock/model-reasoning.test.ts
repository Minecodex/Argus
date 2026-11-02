import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("preserves omitted thinking depth, resets explicitly, and rejects unsupported profiles atomically", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const input = {
    name: "Thinking fixture",
    baseUrl: "https://models.example.test/v1",
    apiKey: "fixture-only-key",
    modelId: "reasoning-model",
    apiProtocol: "chat_completions" as const,
    contextWindowTokens: 131072,
    maxOutputTokens: 8192,
    reasoningEffort: "medium" as const,
    inputPricePerMillionTokens: 1,
    outputPricePerMillionTokens: 2,
  };
  const created = await api.models.testAndCreate(input);
  expect(created.created).toBe(true);
  expect(created.model?.reasoningEffort).toBe("medium");
  expect(created.model).not.toHaveProperty("apiKey");
  const id = created.model!.id;
  expect(
    (await api.models.update(id, { name: "Renamed" })).reasoningEffort,
  ).toBe("medium");
  const before = structuredClone(await api.models.get(id));
  await expect(
    api.models.update(id, {
      apiProtocol: "responses",
      name: "Must not mutate",
    }),
  ).rejects.toThrow("MODEL_COMPATIBILITY_FAILED");
  expect(await api.models.get(id)).toEqual(before);
  const failed = await api.models.testAndCreate({
    ...input,
    apiProtocol: "responses",
  });
  expect(failed.created).toBe(false);
  expect(
    (await api.models.list()).filter((model) => model.name === input.name),
  ).toHaveLength(0);
  const reset = await api.models.update(id, {
    apiProtocol: "responses",
    reasoningEffort: "provider_default",
  });
  expect(reset.reasoningEffort).toBe("provider_default");
  expect(reset.apiProtocol).toBe("responses");
});
