import { describe, expect, it } from "vitest";
import type {
  HostConnectionTestCreate,
  HostPreviewCreate,
} from "../generated/contracts";
import { createMockApiClient } from "./index";

const target: HostConnectionTestCreate = {
  address: "192.0.2.10",
  port: 22,
  platform: "linux",
  ssh_path: "direct_executor",
  credential_id: "sec-ssh-prod",
  username: "root",
};
const host: HostPreviewCreate = {
  ...target,
  name: "callback-probe-host",
  role: "managed_host",
  install_method: "ssh",
  control_path: "direct",
  environment: "production",
  labels: {},
};

async function client() {
  const api = createMockApiClient({ persist: false, delay: 0, stepDelay: 1 });
  await api.auth.login({ username: "root", password: "123456" });
  return api;
}

describe("onboarding callback evidence", () => {
  it("does not reuse a generic SSH test as onboarding proof", async () => {
    const api = await client();
    const test = await api.hosts.createConnectionTest(target);
    expect(test.status).toBe("succeeded");
    expect(
      test.checks.some((check) => check.name === "onboarding_callback"),
    ).toBe(false);
    await expect(
      api.hosts.previewCreateResource({ ...host, connection_test_id: test.id }),
    ).rejects.toMatchObject({ code: "CONNECTION_TEST_REQUIRED" });
  });

  it("freezes callback path and refuses a different path at preview", async () => {
    const api = await client();
    const input: HostConnectionTestCreate = {
      ...target,
      onboarding_control_path: "direct",
    };
    const test = await api.hosts.createConnectionTest(input);
    input.onboarding_control_path = "executor_tunnel";
    expect(test.checks).toContainEqual({
      name: "onboarding_callback",
      status: "passed",
      detail: "direct",
    });
    await expect(
      api.hosts.previewCreateResource({
        ...host,
        control_path: "executor_tunnel",
        connection_test_id: test.id,
      }),
    ).rejects.toMatchObject({ code: "CONNECTION_TEST_REQUIRED" });
    await expect(
      api.hosts.previewRetryResource("existing-host", {
        ...host,
        control_path: "executor_tunnel",
        connection_test_id: test.id,
      }),
    ).rejects.toMatchObject({ code: "CONNECTION_TEST_REQUIRED" });
    await expect(
      api.hosts.previewCreateResource({ ...host, connection_test_id: test.id }),
    ).resolves.toHaveProperty("action_ref");
  });

  it("requires a matching probe for bastion creation and replacement", async () => {
    const api = await client();
    const test = await api.hosts.createConnectionTest({
      ...target,
      onboarding_control_path: "direct",
    });
    const input = {
      ...target,
      name: "callback-bastion",
      install_mode: "direct_install_tunnel" as const,
      environment: "production" as const,
      labels: {},
      connection_test_id: test.id,
    };
    await expect(
      api.connectors.previewCreateBastionScope(input),
    ).rejects.toMatchObject({ code: "CONNECTION_TEST_REQUIRED" });
    const tunnelTest = await api.hosts.createConnectionTest({
      ...target,
      onboarding_control_path: "executor_tunnel",
    });
    const pending = await api.connectors.previewCreateBastionScope({
      ...input,
      connection_test_id: tunnelTest.id,
    });
    await api.approvals.confirm(pending.action_ref);
    const scope = (await api.connectors.listBastionScopes()).items.find(
      (item) => item.name === input.name,
    )!;
    await expect
      .poll(
        async () =>
          (await api.connectors.getBastionScope(scope.id)).active_connector_id,
      )
      .toBeTruthy();
    await expect(
      api.connectors.previewConnectorReplacement(scope.id, {
        ...target,
        expected_version: scope.resource_version,
        connection_test_id: test.id,
      }),
    ).rejects.toMatchObject({ code: "CONNECTION_TEST_REQUIRED" });
  });
});
