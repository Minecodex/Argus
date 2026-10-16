import { expect, it } from "vitest";
import type { AlertSchemas } from "../alerts";
import { createMockApiClient } from "./index";

it("rejects missing update rule identity without preparing an action or changing configuration", async () => {
  const client = createMockApiClient({
    persist: false,
    delay: 0,
    stepDelay: 1,
  });
  await client.auth.login({ username: "root", password: "123456" });
  const policy = (await client.alerts.configurations("policy"))[0];
  if (!policy) throw new Error("seed policy is missing");
  const spec = structuredClone(policy.spec) as AlertSchemas["AlertPolicySpec"];
  const rule = spec.rules[0];
  if (!rule) throw new Error("seed rule is missing");
  delete rule.id;
  const beforeActions = await client.approvals.list();
  await expect(
    client.alerts.preview({
      kind: "policy",
      operation: "update",
      id: policy.id,
      name: policy.name,
      expected_version: policy.version,
      spec,
    }),
  ).rejects.toMatchObject({ status: 400, code: "ALERT_INVALID" });
  expect(await client.alerts.configuration(policy.id)).toEqual(policy);
  expect(await client.approvals.list()).toEqual(beforeActions);
});
