import { describe, expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "./factory";
import { createMockApiClient } from "./mock";
import { createSeedDb } from "./mock/seed";
import { resourceNameAvailable } from "./mock/name-availability";

const hostInput = (name: string) => ({
  name,
  platform: "linux" as const,
  role: "managed_host" as const,
  control_path: "direct" as const,
  install_method: "manual" as const,
  ssh_path: "none" as const,
  architecture: "amd64" as const,
  environment: "production" as const,
  labels: {},
});

describe("resource name availability", () => {
  it("uses enterprise and not-deleted uniqueness including uninstalled names", () => {
    const db = createSeedDb();
    const host = db.hosts[0]!;
    host.name = "unique-policy-name";
    host.status = "uninstalled";
    expect(resourceNameAvailable(db, host.enterpriseId, host.name)).toBe(false);
    expect(resourceNameAvailable(db, "other-enterprise", host.name)).toBe(true);
    host.status = "deleted";
    expect(resourceNameAvailable(db, host.enterpriseId, host.name)).toBe(true);
    const scope = db.bastionScopes[0]!;
    scope.name = "scope-only-name";
    expect(resourceNameAvailable(db, scope.enterpriseId, scope.name)).toBe(
      true,
    );
    expect(
      resourceNameAvailable(db, scope.enterpriseId, scope.name, true),
    ).toBe(false);
    scope.status = "deleted";
    expect(
      resourceNameAvailable(db, scope.enterpriseId, scope.name, true),
    ).toBe(true);
  });

  it("validates Unicode length and normalizes names before freezing a preview", async () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    await client.auth.login({ username: "root", password: "123456" });
    for (const name of ["   ", "a".repeat(129), "😀".repeat(129)]) {
      await expect(
        client.hosts.checkNameAvailability(name),
      ).rejects.toMatchObject({
        status: 400,
        code: "INVALID_ARGUMENT",
        params: { field: "name" },
      });
      await expect(
        client.connectors.checkBastionNameAvailability(name),
      ).rejects.toMatchObject({
        status: 400,
        code: "INVALID_ARGUMENT",
        params: { field: "name" },
      });
      await expect(
        client.hosts.previewCreateResource(hostInput(name)),
      ).rejects.toMatchObject({ code: "INVALID_ARGUMENT" });
    }
    expect(await client.hosts.checkNameAvailability("😀".repeat(128))).toEqual({
      available: true,
    });
    const host = await client.hosts.previewCreateResource(
      hostInput(" padded-name "),
    );
    expect(host.preview).toMatchObject({ name: "padded-name" });
    await client.approvals.confirm(host.action_ref);
    expect(await client.hosts.checkNameAvailability("padded-name")).toEqual({
      available: false,
    });
    const bastion = await client.connectors.previewCreateBastionScope({
      name: " padded-scope ",
      install_mode: "command",
      architecture: "amd64",
      environment: "production",
      labels: {},
    });
    expect(bastion.preview).toMatchObject({ name: "padded-scope" });
  });
  it("uses read-only encoded enterprise routes and preserves boolean results", async () => {
    const fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ available: false }), {
          headers: { "content-type": "application/json" },
        }),
    );
    const client = await createConfiguredApiClient({
      portal: "enterprise",
      mode: "real",
      base_url: "https://api.example.test",
      fetch,
    });
    expect(await client.hosts.checkNameAvailability("node & 一")).toEqual({
      available: false,
    });
    expect(
      await client.connectors.checkBastionNameAvailability("gateway + 一"),
    ).toEqual({ available: false });
    const requests = fetch.mock.calls as unknown as [string, RequestInit][];
    expect(new URL(requests[0]![0]).pathname).toBe(
      "/api/v1/enterprise/hosts/name-availability",
    );
    expect(new URL(requests[0]![0]).searchParams.get("name")).toBe("node & 一");
    expect(new URL(requests[1]![0]).pathname).toBe(
      "/api/v1/enterprise/bastion-scopes/name-availability",
    );
    expect(new URL(requests[1]![0]).searchParams.get("name")).toBe(
      "gateway + 一",
    );
    expect(
      requests.every(([, init]) => !init.method || init.method === "GET"),
    ).toBe(true);
  });

  it("fails closed in a portal that cannot create resources", async () => {
    const client = await createConfiguredApiClient({
      portal: "setup",
      mode: "real",
      base_url: "https://api.example.test",
    });
    await expect(
      client.hosts.checkNameAvailability("name"),
    ).rejects.toMatchObject({ code: "CLIENT_OPERATION_UNAVAILABLE" });
    await expect(
      client.connectors.checkBastionNameAvailability("name"),
    ).rejects.toMatchObject({ code: "CLIENT_OPERATION_UNAVAILABLE" });
  });

  it("checks existing names case-insensitively and rejects duplicate previews", async () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    await client.auth.login({ username: "root", password: "123456" });
    const host = (await client.hosts.list()).items[0]!;
    const scope = (await client.connectors.listBastionScopes()).items[0]!;
    expect(
      await client.hosts.checkNameAvailability(` ${host.name.toUpperCase()} `),
    ).toEqual({ available: false });
    expect(
      await client.connectors.checkBastionNameAvailability(host.name),
    ).toEqual({ available: false });
    expect(
      await client.connectors.checkBastionNameAvailability(
        scope.name.toUpperCase(),
      ),
    ).toEqual({ available: false });
    expect(await client.hosts.checkNameAvailability("unique-new-host")).toEqual(
      { available: true },
    );
    await expect(
      client.hosts.previewCreateResource(hostInput(host.name)),
    ).rejects.toMatchObject({ code: "RESOURCE_NAME_CONFLICT" });
  });

  it("rechecks host and bastion names at confirmation when another creation wins", async () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    await client.auth.login({ username: "root", password: "123456" });
    const host = await client.hosts.previewCreateResource(
      hostInput("concurrent-name"),
    );
    const bastion = await client.connectors.previewCreateBastionScope({
      name: "CONCURRENT-NAME",
      install_mode: "command",
      architecture: "amd64",
      environment: "production",
      labels: {},
    });
    await client.approvals.confirm(host.action_ref);
    await expect(
      client.approvals.confirm(bastion.action_ref),
    ).rejects.toMatchObject({ code: "RESOURCE_NAME_CONFLICT" });
    const second = await client.hosts.previewCreateResource(
      hostInput("another-name"),
    );
    const third = await client.hosts.previewCreateResource(
      hostInput("ANOTHER-NAME"),
    );
    await client.approvals.confirm(second.action_ref);
    await expect(
      client.approvals.confirm(third.action_ref),
    ).rejects.toMatchObject({ code: "RESOURCE_NAME_CONFLICT" });
  });
});
