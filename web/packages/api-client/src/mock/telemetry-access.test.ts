import { expect, it } from "vitest";
import { createSeedDb } from "./seed";
import type { MockContext } from "./context";
import { requireTelemetryResources } from "./telemetry-access";
import {
  builtinRolePermissions,
  permissionIDs,
} from "../generated/permission-registry";

it("uses the canonical object-read roles and fails closed on missing or revoked scope", () => {
  const db = createSeedDb();
  const user = db.enterpriseUsers.find((u) => u.userId === "u-root")!;
  const enterprise = user.enterpriseId;
  const host = db.hosts.find((h) => h.enterpriseId === enterprise)!;
  const cluster = db.clusters.find((c) => c.enterpriseId === enterprise)!;
  const role: typeof db.roles[number] = {
    ...db.roles[0]!,
    id: "object-reader",
    enterprise_id: enterprise,
    permissions: ["host.read"],
    status: "active" as const,
  };
  db.roles = [role];
  db.roleBindings = [
    {
      ...db.roleBindings[0]!,
      role_id: role.id,
      enterprise_id: enterprise,
      subject_type: "user",
      subject_id: user.userId,
      status: "active",
    },
  ];
  db.dataAuthorizationGrants = [
    {
      ...db.dataAuthorizationGrants![0]!,
      subject_type: "user",
      subject_id: user.userId,
      resource_type: "host",
      resource_id: host.id,
      active: true,
    },
    {
      ...db.dataAuthorizationGrants![0]!,
      subject_type: "user",
      subject_id: user.userId,
      resource_type: "kubernetes_cluster",
      resource_id: cluster.id,
      active: true,
    },
  ];
  const ctx = {
    db,
    actor: () => ({ id: user.userId, displayName: "Reader" }),
    enterpriseId: () => enterprise,
    nowIso: () => new Date().toISOString(),
  } as MockContext;
  expect(() => requireTelemetryResources(ctx, [host.id])).not.toThrow();
  expect(() => requireTelemetryResources(ctx, [cluster.id])).toThrow();
  expect(() => requireTelemetryResources(ctx, [host.id, cluster.id])).toThrow();
  expect(() => requireTelemetryResources(ctx, [])).toThrow();
  role.permissions.push("kubernetes.read");
  expect(() => requireTelemetryResources(ctx, [cluster.id])).not.toThrow();
  db.dataAuthorizationGrants![0]!.active = false;
  expect(() => requireTelemetryResources(ctx, [host.id])).toThrow();
  role.status = "disabled";
  expect(() => requireTelemetryResources(ctx, [cluster.id])).toThrow();
});

it("has no per-signal or field privilege in generated built-in roles", () => {
  for (const permissions of Object.values(builtinRolePermissions)) {
    expect(
      permissions.every((id) =>
        permissionIDs.includes(id as (typeof permissionIDs)[number]),
      ),
    ).toBe(true);
    expect(
      permissions.some((id) =>
        /^telemetry\.(query\.|sensitive_fields\.)/.test(id),
      ),
    ).toBe(false);
  }
});
